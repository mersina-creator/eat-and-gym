// Точка сборки: конфиг, логгер, БД, каталог, фоновые синхронизации, сервисы и транспорты.
// Транспорт здесь один — HTTP; очередь или пакетное задание подключаются рядом из тех же service.Services.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os/signal"
	"racion/internal/mail"
	"racion/internal/oauth"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"racion/internal/ai"
	"racion/internal/config"
	"racion/internal/db"
	"racion/internal/geo"
	"racion/internal/localprices"
	"racion/internal/logger"
	"racion/internal/media"
	"racion/internal/messenger"
	"racion/internal/planner"
	"racion/internal/rosstat"
	"racion/internal/seed"
	"racion/internal/service"
	"racion/internal/storage/postgres"
	transport "racion/internal/transport/http"
	"racion/internal/vkusvill"
)

func main() {
	cfg := config.Load()
	ring := logger.NewRing(1000)
	log := logger.WithRing(logger.New(cfg.LogLevel, cfg.LogFormat), ring)
	zap.ReplaceGlobals(log) // сервисы без своего логгера (push, indexnow) пишут через zap.L()
	defer func() { _ = log.Sync() }()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("connect", zap.Error(err))
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal("migrate", zap.Error(err))
	}
	if err := seed.Run(ctx, pool); err != nil {
		log.Fatal("seed", zap.Error(err))
	}
	planner.SetOccasions(seed.Occasions())
	catalog, err := planner.LoadCatalog(ctx, pool)
	if err != nil {
		log.Fatal("catalog", zap.Error(err))
	}
	log.Info("catalog", zap.Int("recipes", len(catalog.Recipes)), zap.Int("ingredients", len(catalog.Ingredients)), zap.Int("stores", len(catalog.StoreList)))
	// цены сетей с открытым каталогом (ВкусВилл): заменяют оценку «Росстат × индекс» для сопоставленных продуктов
	for _, sp := range seed.StorePrices() {
		catalog.SetStorePrices(sp)
		log.Info("store prices", zap.String("store", sp.Store), zap.String("date", sp.Date), zap.Int("items", len(sp.Items)))
	}

	// Страна по IP: база DB-IP в БД, обновление раз в месяц в фоне.
	geoResolver := &geo.Resolver{Dir: cfg.GeoDir}
	geoResolver.Start(ctx, pool, log.Named("geo"))

	// Ценник Росстата: что есть в БД — сразу; обновление — в фоне, раз в сутки, без блокировки старта.
	if pb, err := planner.LoadPriceBook(ctx, pool); err != nil {
		log.Error("pricebook", zap.Error(err))
	} else if pb != nil {
		catalog.SetPriceBook(pb)
		log.Info("pricebook", zap.String("period", pb.Period), zap.Int("regions", len(pb.Regions)))
	}
	go every(ctx, 6*time.Hour, func() {
		if !rosstat.Stale(ctx, pool) {
			return
		}
		sctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if err := rosstat.Sync(sctx, pool, log.Named("rosstat")); err != nil {
			log.Warn("rosstat sync failed, keeping previous prices", zap.Error(err))
			return
		}
		if pb, err := planner.LoadPriceBook(ctx, pool); err == nil && pb != nil {
			catalog.SetPriceBook(pb)
		}
	})

	// Живые цены других стран (BLS, Белстат, Бюро нацстатистики Казахстана, Eurostat, ONS) — тем же способом.
	go every(ctx, 6*time.Hour, func() {
		packs := map[string]localprices.PackInfo{}
		for id, ing := range catalog.Ingredients {
			packs[id] = localprices.PackInfo{Pack: ing.Pack, Unit: ing.Unit, Category: ing.Category, Local: ing.Prices}
		}
		sctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
		if updated := localprices.Sync(sctx, pool, packs, log.Named("localprices")); len(updated) > 0 {
			if fresh, err := planner.LoadCatalog(ctx, pool); err == nil {
				for _, c := range updated {
					if lp := fresh.LocalPrices(c); lp != nil {
						catalog.SetLocalPrices(lp)
					}
				}
			}
		}
	})

	// Сервисы поверх хранилища; транспорты получают их целиком.
	store := postgres.New(pool)

	// Уборка: просроченные сессии и старая аналитика, раз в сутки.
	go every(ctx, 24*time.Hour, func() {
		sctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if n, err := store.Cleanup(sctx, 180*24*time.Hour); err != nil {
			log.Warn("cleanup", zap.Error(err))
		} else if n > 0 {
			log.Info("cleanup", zap.Int64("rows", n))
		}
	})
	// Каталог — через атомарную ссылку: админка после правки рецептов перечитывает его из БД и подменяет целиком.
	catalogRef := planner.NewCatalogRef(catalog)
	services := service.New(service.Repos{
		Users: store.Users, Resets: store.Resets, Sessions: store.Sessions, Plans: store.Plans, Dislikes: store.Dislikes, Checks: store.Checks,
		Purchases: store.Purchases, Extras: store.Extras, UserRecipes: store.UserRecipes, Events: store.Events,
		PlanMembers: store.PlanMembers, Push: store.Push, Settings: store.Settings, Social: store.Social, Households: store.Households, Admin: store.Admin, Collections: store.Collections, Partners: store.Partners, Offers: store.Offers, APIKeys: store.APIKeys, Profiles: store.Profiles,
	}, catalogRef, cfg.PushContact, cfg.BaseURL)
	services.Admin = service.NewAdmin(store.Admin, store.Users, cfg.AdminEmails)
	// письма: восстановление пароля; без MAIL_HOST письмо только в логе (локальный стенд)
	oauthReg := oauth.New(oauth.Config(cfg.OAuth))
	if en := oauthReg.Enabled(); len(en) > 0 {
		log.Info("oauth on", zap.Strings("providers", en))
	}
	services.Accounts.SetAvatarImporter(func(ctx context.Context, userID, src string) (string, error) {
		p, err := services.Media.Import(ctx, userID, "avatar", src)
		return p.URL, err
	})
	services.Accounts.SetMailer(mail.New(mail.Config{Host: cfg.MailHost, Port: cfg.MailPort, User: cfg.MailUser, Pass: cfg.MailPass, From: cfg.MailFrom}, log.Named("mail")))
	// замены продуктов: таблица из seed/data/substitutes.json
	subsTable := map[string][]service.SubEntry{}
	for id, list := range seed.Substitutes() {
		for _, e := range list {
			subsTable[id] = append(subsTable[id], service.SubEntry{ID: e.ID, Ratio: e.Ratio, Note: e.Note, NoteEn: e.NoteEn, Not: e.Not})
		}
	}
	services.Subs = service.NewSubstitutes(subsTable, services.Recipes)
	// корзина ВкусВилла одной ссылкой: их MCP открыт без ключа (см. internal/vkusvill)
	vv := vkusvill.New(cfg.VkusvillMCP)
	services.Plans.SetCart(vv)
	// цены ВкусВилла: сопоставление из данных проекта, цена и вес каждого товара — сверка раз в сутки
	units := make(map[string]string, len(catalog.Ingredients))
	for id, ing := range catalog.Ingredients {
		units[id] = ing.Unit
	}
	for _, sp := range seed.StorePrices() {
		if sp.Store != "vkusvill" {
			continue
		}
		prices := service.NewStorePrices(store.StorePrices, vv, sp, units, catalog.SetStorePrices, log.Named("storeprices"))
		if err := prices.Load(ctx); err != nil {
			log.Warn("store prices load", zap.Error(err))
		}
		go every(ctx, 6*time.Hour, func() {
			sctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			defer cancel()
			if n, err := prices.Refresh(sctx); err != nil {
				log.Warn("store prices refresh", zap.Error(err))
			} else if n > 0 {
				log.Info("store prices checked", zap.String("store", sp.Store), zap.Int("products", n))
			}
		})
	}
	// боты в Telegram и MAX: список покупок по отделам; без токена мессенджер выключен
	services.Bots = service.NewBots(service.BotDeps{Repo: store.Messenger, Plans: services.Plans, Accounts: services.Accounts, Taste: services.Social, Journal: store.Push},
		cfg.BaseURL, log.Named("bots"))
	if b := cfg.Bots; b.TelegramToken != "" && b.TelegramName != "" {
		tg := messenger.NewTelegram(b.TelegramToken, b.TelegramName, botSecret(b.Secret, b.TelegramToken))
		if b.TelegramAPI != "" {
			tg.SetBase(b.TelegramAPI)
		}
		// опросом по умолчанию: запросы с адресов Telegram до сервера в России доходят через раз
		services.Bots.Add(tg, b.TelegramUpdates != "webhook")
	}
	if b := cfg.Bots; b.MaxToken != "" && b.MaxName != "" {
		mx := messenger.NewMax(b.MaxToken, b.MaxName, botSecret(b.Secret, b.MaxToken))
		if b.MaxAPI != "" {
			mx.SetBase(b.MaxAPI)
		}
		services.Bots.Add(mx, false)
	}
	go services.Bots.Run(ctx)
	// Рецепты базы из админки: после правки каталог перечитывается из БД с теми же ценниками
	reloadCatalog := func(ctx context.Context) error {
		fresh, err := catalogRef.Load().Reload(ctx, pool)
		if err != nil {
			return err
		}
		catalogRef.Store(fresh)
		return nil
	}
	services.CatalogAdmin = service.NewCatalogAdmin(store.CatalogRecipes, catalogRef, reloadCatalog)
	// Нейросети: пул провайдеров (бесплатные уровни Mistral/Gemini/Groq/OpenRouter, OpenAI, локальный прокси);
	// один пул на модерацию, помощника и очередь переводов своих рецептов
	aiPool := ai.NewPoolFromKeys(ai.ProviderKeys{Order: splitList(cfg.AIOrder), Mistral: cfg.MistralKey, Gemini: cfg.GeminiKey, Groq: cfg.GroqKey, OpenRouter: cfg.OpenRouterKey, OpenAI: cfg.OpenAIKey, LocalURL: cfg.LocalAIURL, Models: parseModels(cfg.AIModels, cfg.OpenAIModel)})
	var checker service.RecipeChecker
	if aiPool.Enabled() {
		checker = aiPool
	}
	services.Moderation = service.NewModeration(store.UserRecipes, store.UserRecipes, store.Users, checker, log.Named("moderation"))
	services.Translations = service.NewTranslations(store.Translations, store.UserRecipes, store.UserRecipes, aiPool, log.Named("translations"))
	services.Translations.SetCatalog(catalogRef, store.CatalogRecipes, reloadCatalog)
	services.Recipes.SetTranslations(services.Translations)
	services.PlanChat = service.NewPlanChat(aiPool, services.Plans)
	go services.Translations.Run(ctx)
	if err := services.Partners.Seed(ctx); err != nil {
		log.Warn("partners seed", zap.Error(err))
	}
	// Фото в S3/MinIO: без S3_ENDPOINT загрузка выключена, всё остальное работает
	mediaStore, err := media.New(media.Config{Endpoint: cfg.S3Endpoint, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, Bucket: cfg.S3Bucket, Secure: cfg.S3Secure, PublicURL: cfg.S3PublicURL})
	if err != nil {
		log.Warn("media", zap.Error(err))
	} else if mediaStore != nil {
		if err := mediaStore.Init(ctx); err != nil {
			log.Warn("media init", zap.Error(err))
		} else {
			services.Media = service.NewMedia(mediaStore)
			services.Accounts.SetMedia(services.Media.Owns)
			services.Recipes.SetMedia(services.Media.Owns)
			services.Social.SetMedia(services.Media.Owns)
			log.Info("media on", zap.String("bucket", cfg.S3Bucket))
		}
	}
	if aiPool.Enabled() {
		services.AI = service.NewAssistant(aiPool)
		names := []string{}
		for _, p := range aiPool.Status() {
			names = append(names, p.Name+"/"+p.Model)
		}
		log.Info("ai on", zap.Strings("providers", names))
	}

	// Push: ключи VAPID и проход по напоминаниям раз в 10 минут.
	services.Notify.SetDigest(func(ctx context.Context) (int, int) {
		r, c, _ := store.Admin.NewSince(ctx, time.Now().AddDate(0, 0, -7))
		return r, c
	})
	if err := services.Notify.Init(ctx); err != nil {
		log.Warn("push init", zap.Error(err))
	}
	go every(ctx, 10*time.Minute, func() {
		now := time.Now()
		// сначала чаты ботов: привязанному аккаунту напоминание уходит в Telegram и отмечается в журнале
		// веб-пуша, который следом его пропустит
		bctx, bcancel := context.WithTimeout(ctx, 5*time.Minute)
		if n, err := services.Bots.Tick(bctx, now); err != nil {
			log.Warn("bots tick", zap.Error(err))
		} else if n > 0 {
			log.Info("bots reminders sent", zap.Int("count", n))
		}
		bcancel()
		sctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if n, err := services.Notify.Tick(sctx, now); err != nil {
			log.Warn("push tick", zap.Error(err))
		} else if n > 0 {
			log.Info("push sent", zap.Int("count", n))
		}
	})

	// монитор для /status: база и почта каждую минуту, хранилище фото — если включено
	monitor := service.NewHealth(postgres.NewHealth(pool), log.Named("health"))
	monitor.AddCheck(service.Check{Key: "db", Fn: store.Ping})
	if mediaStore != nil && services.Media != nil {
		monitor.AddCheck(service.Check{Key: "storage", Fn: mediaStore.Ping})
	}
	if cfg.MailHost != "" {
		monitor.AddCheck(service.Check{Key: "mail", Fn: service.TCPCheck(net.JoinHostPort(cfg.MailHost, cfg.MailPort)), Every: 5 * time.Minute})
	}
	monitor.AddStatic("push", services.Notify.PublicKey() != "", "")
	monitor.AddStatic("search", services.IndexNow != nil && services.IndexNow.Key(ctx) != "", "")
	go monitor.Run(ctx)
	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: transport.New(transport.Deps{
			Services: services, Log: log.Named("http"), Geo: geoResolver,
			Health:  func() error { return store.Ping(context.Background()) },
			Monitor: monitor,
			OAuth:   oauthReg,
			BaseURL: cfg.BaseURL,
			Metrika: cfg.MetrikaID, Contact: cfg.LegalEmail, Images: cfg.ImagesDir,
			Logs: ring, Quota: store.APIUsage,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		log.Info("listen", zap.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("serve", zap.Error(err))
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// every запускает fn сразу и затем по тикеру, пока не отменён контекст.
func every(ctx context.Context, d time.Duration, fn func()) {
	fn()
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}

// botSecret — секрет вебхука бота: из BOT_SECRET или, если он пуст, из токена бота. Мессенджеры
// принимают только A-Z, a-z, 0-9, «_» и «-», поэтому hex.
func botSecret(secret, token string) string {
	if secret != "" {
		return secret
	}
	m := hmac.New(sha256.New, []byte(token))
	m.Write([]byte("racion webhook"))
	return hex.EncodeToString(m.Sum(nil))
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// parseModels — «mistral=ministral-14b-latest,gemini=…» → map; OPENAI_MODEL — как раньше.
func parseModels(s, openai string) map[string]string {
	m := map[string]string{"openai": openai}
	for _, kv := range splitList(s) {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}
