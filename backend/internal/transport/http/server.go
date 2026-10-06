// Package http — HTTP-транспорт: роутер на stdlib, JSON-ручки и HTML-страницы. Тонкий слой: разобрать запрос,
// вызвать сервис, отдать ответ. Правила и SQL живут в service и storage, так что другой транспорт
// (очередь, пакетное задание) собирается рядом из тех же сервисов.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"racion/internal/domain"
	"racion/internal/geo"
	"racion/internal/i18n"
	"racion/internal/logger"
	"racion/internal/oauth"
	"racion/internal/planner"
	"racion/internal/service"
	"racion/locales"
)

type Server struct {
	svc       *service.Services
	catalog   *planner.Catalog // общий каталог для страниц и справочников
	log       *zap.Logger
	geo       *geo.Resolver
	health    func() error
	monitor   *service.Health
	lim       *limits
	logs      *logger.Ring
	oauth     *oauth.Registry
	publicURL string     // публичный адрес для canonical и sitemap; пусто — по заголовкам запроса
	pages     *pageCache // микрокэш готовых страниц каталога
	quota     APIQuota   // квота стороннего API (quota.go)
}

// Deps — всё, что нужно транспорту от приложения.
type Deps struct {
	Services *service.Services
	Log      *zap.Logger
	Geo      *geo.Resolver
	Health   func() error    // проверка живости хранилища для /healthz
	Monitor  *service.Health // страница /status
	BaseURL  string          // например https://racion.app; пусто — брать из запроса
	Metrika  string          // id счётчика Яндекс Метрики для SSR-страниц
	Contact  string          // почта для юридических страниц (LEGAL_EMAIL)
	Images   string          // каталог с фото блюд (том фронтенда) для карточек превью
	Logs     *logger.Ring    // последние записи лога для админки
	OAuth    *oauth.Registry
	Quota    APIQuota // квота стороннего API; nil — без квоты (тесты)
}

func New(d Deps) http.Handler {
	metrikaID = d.Metrika
	legalEmail = d.Contact
	imagesDir = d.Images
	featuredFn = func(l i18n.Lang, p string) []FootLink {
		var out []FootLink
		for _, slug := range featuredSlugs {
			for _, c := range d.Services.Collections.Curated(context.Background()) {
				if c.Slug == slug && c.Public {
					c = c.Localized(string(l))
					out = append(out, FootLink{Name: c.Name, Href: p + "/collection/" + c.Slug})
					break
				}
			}
		}
		if topicLang(l) { // недельные меню и «из продуктов» есть только на ru/en/de
			out = append(out, FootLink{Name: i18n.T(l, "menu.family-4.h1"), Href: p + "/menu/family-4"}, FootLink{Name: i18n.T(l, "cook.hub.title"), Href: p + "/recipes/from"})
		}
		return out
	}
	s := &Server{svc: d.Services, catalog: d.Services.Catalog.Base(), log: d.Log, geo: d.Geo, health: d.Health, monitor: d.Monitor, lim: newLimits(), publicURL: strings.TrimRight(d.BaseURL, "/"), logs: d.Logs, oauth: d.OAuth, pages: newPageCache(), quota: d.Quota}
	mux := http.NewServeMux()
	// Публичные методы, описанные в openapi.json, вызываются в том числе из браузера: без CORS
	// стороннее приложение до них не достучится. Куки при этом не передаются — доступ анонимный.
	cors := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Expose-Headers", quotaHeaders)
			w.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("OPTIONS /api/plans", cors(nil))
	mux.HandleFunc("OPTIONS /api/baskets", cors(nil))
	mux.HandleFunc("OPTIONS /api/occasions/{id}", cors(nil))
	mux.HandleFunc("GET /api/meta", cors(s.meta))
	mux.HandleFunc("POST /api/plans", cors(s.limited(s.lim.build, s.createPlan)))
	mux.HandleFunc("GET /api/plans/{id}", cors(s.getPlan))
	mux.HandleFunc("POST /api/plans/{id}/swap", s.limited(s.lim.build, s.swap))
	mux.HandleFunc("POST /api/plans/{id}/side", s.limited(s.lim.build, s.swapSide))
	mux.HandleFunc("POST /api/plans/{id}/chat", s.limited(s.lim.build, s.planChat))
	mux.HandleFunc("POST /api/plans/{id}/skip", s.limited(s.lim.write, s.skipDay))
	mux.HandleFunc("POST /api/plans/{id}/move", s.limited(s.lim.write, s.moveDish))
	mux.HandleFunc("POST /api/plans/{id}/repeat", s.limited(s.lim.build, s.repeatPlan))
	mux.HandleFunc("GET /api/plans/{id}/checks", s.planChecks)
	mux.HandleFunc("PUT /api/plans/{id}/checks", s.limited(s.lim.write, s.setCheck))
	mux.HandleFunc("GET /api/plans/{id}/extras", s.listExtras)
	mux.HandleFunc("POST /api/plans/{id}/extras", s.limited(s.lim.write, s.addExtra))
	mux.HandleFunc("DELETE /api/plans/{id}/extras/{extra}", s.deleteExtra)
	mux.HandleFunc("POST /api/plans/{id}/join", s.limited(s.lim.write, s.joinPlan))
	mux.HandleFunc("PATCH /api/plans/{id}", s.renamePlan)
	mux.HandleFunc("DELETE /api/plans/{id}", s.deletePlan)
	mux.HandleFunc("GET /api/recipes", cors(s.recipesAPI))
	mux.HandleFunc("GET /api/recipes/{id}", cors(s.recipe))
	mux.HandleFunc("GET /api/recipes/{id}/stats", s.recipeStats)
	mux.HandleFunc("POST /api/recipes/{id}/rating", s.limited(s.lim.write, s.rateRecipe))
	mux.HandleFunc("PUT /api/recipes/{id}/like", s.limited(s.lim.write, s.setLike(true)))
	mux.HandleFunc("DELETE /api/recipes/{id}/like", s.setLike(false))
	mux.HandleFunc("PUT /api/recipes/{id}/favorite", s.limited(s.lim.write, s.setFavorite(true)))
	mux.HandleFunc("POST /api/recipes/{id}/feedback", s.limited(s.lim.write, s.setFeedback))
	mux.HandleFunc("GET /api/recipes/{id}/subs", s.recipeSubs)
	mux.HandleFunc("GET /api/occasions", cors(s.occasions))
	mux.HandleFunc("POST /api/occasions/{id}", cors(s.limited(s.lim.build, s.createOccasion)))
	mux.HandleFunc("POST /api/baskets", cors(s.limited(s.lim.build, s.createBasket)))
	mux.HandleFunc("POST /api/plans/{id}/cart", s.limited(s.lim.build, s.storeCart)) // корзина ВкусВилла ссылкой
	mux.HandleFunc("POST /api/bots/{platform}", s.botUpdate)                         // вебхуки Telegram и MAX
	// вход в мини-приложении — не под /api/auth/: подпись мессенджера не подобрать, а открытий много,
	// и строгий лимит на вход по паролю задевал бы людей за одним адресом мобильного оператора
	mux.HandleFunc("POST /api/webapp/{platform}/auth", s.limited(s.lim.write, s.webAppAuth))
	mux.HandleFunc("POST /api/webapp/{platform}/list", s.limited(s.lim.write, s.webAppList))
	mux.HandleFunc("POST /api/me/messengers/{platform}", s.limited(s.lim.write, s.linkMessenger))
	mux.HandleFunc("DELETE /api/me/messengers/{platform}", s.limited(s.lim.write, s.unlinkMessenger))
	mux.HandleFunc("GET /api/collections", cors(s.publicCollections))
	mux.HandleFunc("GET /api/collections/featured", s.featuredCollections) // подвал приложения: четыре подборки, а не весь список
	mux.HandleFunc("PUT /api/me/collections/{id}/public", s.limited(s.lim.write, s.publishCollection))
	mux.HandleFunc("GET /api/admin/collections", s.adminCollections)
	mux.HandleFunc("POST /api/admin/collections", s.limited(s.lim.write, s.adminSaveCollection))
	mux.HandleFunc("GET /api/partners", s.partners)
	mux.HandleFunc("GET /llms.txt", s.llmsTxt)
	mux.HandleFunc("GET /openapi.json", s.openapiJSON)
	mux.HandleFunc("GET /.well-known/ard.json", s.ardManifest)
	mux.HandleFunc("GET /.well-known/ai-catalog.json", s.ardManifest)
	mux.HandleFunc("GET /api/admin/partners", s.adminPartners)
	mux.HandleFunc("POST /api/admin/partners", s.limited(s.lim.write, s.adminSavePartner))
	mux.HandleFunc("DELETE /api/admin/partners/{code}", s.adminDeletePartner)
	mux.HandleFunc("GET /api/offers", s.offers)
	mux.HandleFunc("GET /api/admin/ads", s.adminAds)
	mux.HandleFunc("PUT /api/admin/ads", s.adminAds)
	mux.HandleFunc("GET /api/admin/offers", s.adminOffers)
	mux.HandleFunc("POST /api/admin/offers", s.limited(s.lim.write, s.adminSaveOffer))
	mux.HandleFunc("DELETE /api/admin/offers/{id}", s.adminDeleteOffer)
	mux.HandleFunc("DELETE /api/admin/collections/{id}", s.adminDeleteCollection)
	mux.HandleFunc("GET /collection/{slug}", s.collectionPage)
	mux.HandleFunc("GET /recipes/from", s.cookHubPage)
	mux.HandleFunc("GET /weaning", s.weaningPage)
	mux.HandleFunc("GET /recipes/from/{slug}", s.cookPage)
	mux.HandleFunc("GET /menu/{slug}", s.menuPage)
	mux.HandleFunc("GET /collections", s.collectionsPage)
	// превью ссылок для ботов мессенджеров на страницы приложения (nginx проксирует сюда по User-Agent)
	mux.HandleFunc("GET /og/", s.ogHome)
	mux.HandleFunc("GET /og/plan/{id}", s.ogPlan)
	mux.HandleFunc("GET /og/event/{id}", s.ogEvent)
	mux.HandleFunc("GET /og/recipe/{id}", s.ogRecipe)
	mux.HandleFunc("GET /og/collection/{slug}", s.ogCollection)
	mux.HandleFunc("GET /api/me/collections", s.myCollections)
	mux.HandleFunc("POST /api/me/collections", s.limited(s.lim.write, s.createCollection))
	mux.HandleFunc("PUT /api/me/collections/{id}", s.limited(s.lim.write, s.renameCollection))
	mux.HandleFunc("DELETE /api/me/collections/{id}", s.deleteCollection)
	mux.HandleFunc("GET /api/me/collections/{id}/items", s.collectionItems)
	mux.HandleFunc("PUT /api/me/collections/{id}/items/{recipe}", s.limited(s.lim.write, s.toggleCollectionItem(true)))
	mux.HandleFunc("DELETE /api/me/collections/{id}/items/{recipe}", s.limited(s.lim.write, s.toggleCollectionItem(false)))
	mux.HandleFunc("DELETE /api/recipes/{id}/favorite", s.setFavorite(false))
	mux.HandleFunc("GET /api/recipes/{id}/comments", s.listComments)
	mux.HandleFunc("POST /api/recipes/{id}/comments", s.limited(s.lim.write, s.addComment))
	mux.HandleFunc("DELETE /api/comments/{comment}", s.deleteComment)
	mux.HandleFunc("GET /api/me/favorites", s.myFavorites)
	mux.HandleFunc("PUT /api/me/recipes/{id}/public", s.limited(s.lim.write, s.setOwnPublic))
	mux.HandleFunc("GET /api/me/recipes/{id}/translations", s.ownTranslations)
	mux.HandleFunc("POST /api/me/recipes/{id}/translations", s.limited(s.lim.write, s.ownTranslate))
	mux.HandleFunc("GET /api/admin/ai", s.adminAI)
	mux.HandleFunc("GET /api/ingredients", cors(s.ingredientsAPI))
	mux.HandleFunc("POST /api/events", s.limited(s.lim.events, s.events))
	// аккаунт
	mux.HandleFunc("POST /api/auth/register", s.limited(s.lim.auth, s.register))
	mux.HandleFunc("POST /api/auth/login", s.limited(s.lim.auth, s.login))
	mux.HandleFunc("GET /api/auth/providers", s.oauthProviders)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/start", s.limited(s.lim.auth, s.oauthStart))
	mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", s.oauthCallback)
	mux.HandleFunc("POST /api/auth/oauth/{provider}/callback", s.oauthCallback)
	mux.HandleFunc("POST /api/auth/forgot", s.limited(s.lim.auth, s.forgot))
	mux.HandleFunc("POST /api/auth/reset", s.limited(s.lim.auth, s.reset))
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("PATCH /api/me", s.updateMe)
	mux.HandleFunc("GET /api/me/plans", s.myPlans)
	mux.HandleFunc("GET /api/me/dislikes", s.listDislikes)
	mux.HandleFunc("PUT /api/me/dislikes/{recipe}", s.addDislike)
	mux.HandleFunc("DELETE /api/me/dislikes/{recipe}", s.removeDislike)
	mux.HandleFunc("GET /api/me/purchases", s.myPurchases)
	mux.HandleFunc("GET /api/me/budget", s.myBudget)
	mux.HandleFunc("GET /api/admin/overview", s.adminOverview)
	mux.HandleFunc("GET /api/admin/errors", s.adminErrors)
	mux.HandleFunc("GET /api/admin/users", s.adminUsers)
	mux.HandleFunc("GET /api/admin/logs", s.adminLogs)
	mux.HandleFunc("GET /api/admin/recipes", s.adminRecipes)
	mux.HandleFunc("GET /api/admin/recipes/{id}", s.adminRecipe)
	mux.HandleFunc("POST /api/admin/recipes", s.limited(s.lim.write, s.adminSaveRecipe))
	mux.HandleFunc("POST /api/admin/recipes/batch", s.limited(s.lim.write, s.adminSaveRecipesBatch))
	mux.HandleFunc("POST /api/admin/recipes/{id}/photo", s.limited(s.lim.write, s.adminRecipePhoto))
	mux.HandleFunc("POST /api/admin/recipes/{id}/publish", s.limited(s.lim.write, s.adminRecipePublish))
	mux.HandleFunc("POST /api/admin/recipes/translate-missing", s.limited(s.lim.write, s.adminTranslateMissing))
	mux.HandleFunc("DELETE /api/admin/recipes/{id}/publish", s.limited(s.lim.write, s.adminRecipePublish))
	mux.HandleFunc("GET /api/admin/recipes/schema", s.adminRecipeSchema)
	mux.HandleFunc("GET /api/admin/ingredients", s.adminIngredients)
	mux.HandleFunc("GET /api/me/keys", s.apiKeys)
	mux.HandleFunc("POST /api/me/keys", s.limited(s.lim.write, s.apiKeyCreate))
	mux.HandleFunc("DELETE /api/me/keys/{id}", s.apiKeyDelete)
	mux.HandleFunc("DELETE /api/admin/recipes/{id}", s.adminDeleteRecipe)
	mux.HandleFunc("GET /api/admin/moderation", s.adminModeration)
	mux.HandleFunc("POST /api/admin/moderation/{id}", s.limited(s.lim.write, s.adminDecide))
	mux.HandleFunc("PUT /api/admin/users/{id}/role", s.limited(s.lim.write, s.adminSetRole))
	mux.HandleFunc("POST /api/me/recipes/{id}/publish", s.limited(s.lim.write, s.publishOwn))
	mux.HandleFunc("POST /api/me/recipes/{id}/suggestion", s.limited(s.lim.write, s.suggestionOwn))
	mux.HandleFunc("GET /api/me/recipes", s.listOwnRecipes)
	mux.HandleFunc("POST /api/me/recipes", s.limited(s.lim.write, s.createOwnRecipe))
	mux.HandleFunc("PUT /api/me/recipes/{id}", s.limited(s.lim.write, s.updateOwnRecipe))
	mux.HandleFunc("DELETE /api/me/recipes/{id}", s.deleteOwnRecipe)
	mux.HandleFunc("POST /api/me/recipes/ai", s.limited(s.lim.write, s.assistRecipe))
	mux.HandleFunc("POST /api/uploads", s.limited(s.lim.write, s.upload))
	// семья
	mux.HandleFunc("GET /api/me/family", s.getFamily)
	mux.HandleFunc("PUT /api/me/family", s.limited(s.lim.write, s.saveFamily))
	mux.HandleFunc("POST /api/me/family/invite", s.limited(s.lim.write, s.familyInvite))
	mux.HandleFunc("POST /api/me/family/join", s.limited(s.lim.write, s.familyJoin))
	mux.HandleFunc("POST /api/me/family/leave", s.familyLeave)
	mux.HandleFunc("DELETE /api/me/family/accounts/{user}", s.familyRemove)
	// push
	mux.HandleFunc("GET /api/push/key", s.pushKey)
	mux.HandleFunc("POST /api/me/push", s.limited(s.lim.write, s.pushSubscribe))
	mux.HandleFunc("DELETE /api/me/push", s.pushUnsubscribe)
	// ФТ-01: профиль и норма калорий/БЖУ
	mux.HandleFunc("GET /api/me/profile", s.getProfile)
	mux.HandleFunc("PUT /api/me/profile", s.limited(s.lim.write, s.putProfile))
	// ФТ-05: журнал веса
	mux.HandleFunc("GET /api/me/journal/weight", s.weightJournal)
	mux.HandleFunc("PUT /api/me/weight/{date}", s.limited(s.lim.write, s.setWeight))
	mux.HandleFunc("DELETE /api/me/weight/{date}", s.limited(s.lim.write, s.deleteWeight))
	mux.HandleFunc("POST /api/me/weight/import", s.limited(s.lim.write, s.importWeight))
	mux.HandleFunc("GET /api/me/notify", s.notifySettings)
	mux.HandleFunc("PUT /api/me/notify", s.limited(s.lim.write, s.setNotifySettings))
	mux.HandleFunc("POST /api/me/notify/test", s.limited(s.lim.auth, s.notifyTest))
	mux.HandleFunc("POST /api/me/notify/test/result", s.notifyTestResult)
	// страницы для людей и поисковиков
	mux.HandleFunc("GET /recipes", s.recipesPage)
	mux.HandleFunc("GET /recipe/{id}", s.recipePage)
	// /ru/... — тот же русский без префикса: постоянный редирект, чтобы у страницы был один адрес
	mux.HandleFunc("GET /ru/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.TrimPrefix(r.URL.Path, "/ru")+queryOf(r), http.StatusMovedPermanently)
	})
	for _, l := range i18n.Langs { // /en/recipes, /pl/recipe/{id} — для каждого языка из locales/
		if l == i18n.RU {
			continue
		}
		mux.HandleFunc("GET /"+string(l)+"/recipes", s.recipesPage)
		mux.HandleFunc("GET /"+string(l)+"/recipe/{id}", s.recipePage)
		mux.HandleFunc("GET /"+string(l)+"/collection/{slug}", s.collectionPage)
		mux.HandleFunc("GET /"+string(l)+"/collections", s.collectionsPage)
		mux.HandleFunc("GET /"+string(l)+"/recipes/from", s.cookHubPage)
		mux.HandleFunc("GET /"+string(l)+"/weaning", s.weaningPage)
		mux.HandleFunc("GET /"+string(l)+"/recipes/from/{slug}", s.cookPage)
		mux.HandleFunc("GET /"+string(l)+"/menu/{slug}", s.menuPage)
		mux.HandleFunc("GET /"+string(l)+"/developers", s.devPage)
		mux.HandleFunc("GET /"+string(l)+"/terms", s.legalPage)
		mux.HandleFunc("GET /"+string(l)+"/privacy", s.legalPage)
		mux.HandleFunc("GET /"+string(l)+"/status", s.statusPage)
	}
	mux.HandleFunc("GET /developers", s.devPage)
	mux.HandleFunc("GET /terms", s.legalPage)
	mux.HandleFunc("GET /privacy", s.legalPage)
	mux.HandleFunc("GET /status", s.statusPage)
	mux.HandleFunc("GET /api/status", s.statusAPI)
	mux.HandleFunc("GET /api/locales", s.localesList)
	mux.HandleFunc("GET /api/lang", s.langHint)
	mux.HandleFunc("GET /api/locales/{code}", s.localeFile)
	mux.HandleFunc("GET /sitemap.xml", s.sitemap)
	mux.HandleFunc("GET /sitemap/{file}", s.sitemapLang) // /sitemap/ru.xml … по языку
	mux.HandleFunc("GET /{file}", s.indexNowKey)         // /<key>.txt в корне: ключ IndexNow действует на весь сайт только из корня
	mux.HandleFunc("GET /robots.txt", s.robots)
	return s.withLogging(s.withRecover(s.withHeaders(s.withUser(s.withLimit(s.withPageCache(mux)))))) // пользователь известен до лимита: админы без лимитов
}

// ── Middleware ─────────────────────────────────────────────────────────────

// withHeaders — ответы API не кэшируются (в них сессия и личные данные); защитные заголовки ставит nginx.
func queryOf(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}

// withRecover — паника в обработчике не роняет процесс: API получает JSON 500, страницы — оформленную 500.
func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic", zap.Any("err", rec), zap.String("path", r.URL.Path), zap.Stack("stack"))
				if strings.HasPrefix(r.URL.Path, "/api/") {
					writeErr(w, 500, i18n.T(i18n.FromRequest(r), "api.server"))
				} else {
					s.errorPage(w, r, 500)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			// HTML-страницы зависят от сессии (лайки, комментарии, ник): браузер и прокси перепроверяют
			w.Header().Set("Cache-Control", "private, no-cache")
			w.Header().Add("Vary", "Cookie")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		s.log.Info("http", zap.String("method", r.Method), zap.String("path", r.URL.Path), zap.Int("status", rw.status), zap.Duration("took", time.Since(start)))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// ── Ответы ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decode читает JSON-тело с лимитом размера.
func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v); err != nil {
		writeErr(w, 400, "bad json")
		return false
	}
	return true
}

// fail переводит ошибку сервиса в статус и текст на языке запроса.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	lang := i18n.FromRequest(r)
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		writeErr(w, 422, i18n.T(lang, ve.Key))
	case errors.Is(err, domain.ErrNotFound):
		writeErr(w, 404, "not found")
	case errors.Is(err, domain.ErrUnauthorized):
		writeErr(w, 401, i18n.T(lang, "auth.required"))
	case errors.Is(err, domain.ErrForbidden):
		writeErr(w, 403, i18n.T(lang, "plan.readonly"))
	case errors.Is(err, domain.ErrConflict):
		writeErr(w, 409, "conflict")
	case errors.Is(err, domain.ErrBadInput):
		writeErr(w, 400, "bad json")
	default:
		s.log.Error("request failed", zap.String("path", r.URL.Path), zap.Error(err))
		writeErr(w, 500, "db")
	}
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if s.health != nil {
		if err := s.health(); err != nil {
			writeErr(w, 503, "db")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "recipes": s.visibleRecipes()})
}

// visibleRecipes — рецепты базы без скрытых: то, что видят каталог и планировщик.
func (s *Server) visibleRecipes() int {
	n := 0
	for _, rc := range s.catalog.Recipes {
		if !rc.Hidden {
			n++
		}
	}
	return n
}

// geoCountry — страна посетителя по IP (или по заголовку CF-IPCountry за Cloudflare), только если она
// среди поддерживаемых; иначе пусто, и клиент подставит страну по языку.
func (s *Server) geoCountry(r *http.Request) string {
	code := r.Header.Get("CF-IPCountry")
	if code == "" && s.geo != nil {
		code = s.geo.Country(geo.ClientIP(r))
	}
	if _, ok := countryValid(code); ok {
		return code
	}
	return ""
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	country := planner.CountryOf(r.URL.Query().Get("country"))
	m := s.svc.Catalog.Meta(lang, country, s.geoCountry(r))
	m.GeoLang = geoLang(s.rawGeoCountry(r))
	if s.geo != nil && country.HasRegions {
		if pl := s.geo.Place(geo.ClientIP(r)); pl.Country == country.Code {
			m.GeoRegion = planner.RegionByPlace(m.Regions, pl.Region, pl.City)
		}
	}
	m.AI = s.svc.AI.Enabled()
	m.Photos = s.svc.Media.Enabled()
	m.Bots = s.svc.Bots.Names()
	writeJSON(w, 200, m)
}

// rawGeoCountry — страна по IP без проверки «поддерживаем ли»: нужна для выбора языка.
func (s *Server) rawGeoCountry(r *http.Request) string {
	if code := r.Header.Get("CF-IPCountry"); code != "" {
		return code
	}
	if s.geo != nil {
		return s.geo.Country(geo.ClientIP(r))
	}
	return ""
}

// geoLang — язык по стране посетителя, если такой язык есть в locales/.
func geoLang(country string) string { return i18n.LangByCountry(country) }

// langHint — подсказка языка по стране посетителя (без кэша: зависит от IP). country — сама страна,
// любая, не только из поддерживаемых: по ней баннер cookies решает, показывать ли выбор для ЕС.
func (s *Server) langHint(w http.ResponseWriter, r *http.Request) {
	country := s.rawGeoCountry(r)
	writeJSON(w, 200, map[string]string{"geo": geoLang(country), "accept": string(i18n.FromAccept(r.Header.Get("Accept-Language"))), "country": country})
}

// localesList — языки для переключателя: код, название, флаг, полнота перевода.
func (s *Server) localesList(w http.ResponseWriter, r *http.Request) {
	out := make([]locales.Meta, 0, len(locales.Order))
	for _, c := range locales.Order {
		out = append(out, locales.All[c].Meta)
	}
	w.Header().Set("Cache-Control", "no-cache") // список маленький, а версии словарей должны обновляться сразу после выкладки
	writeJSON(w, 200, out)
}

// localeFile — словарь языка как есть (JSON из locales/), с кэшем на час.
func (s *Server) localeFile(w http.ResponseWriter, r *http.Request) {
	loc, ok := locales.All[r.PathValue("code")]
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(loc.Raw)
}

func (s *Server) ingredientsAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.svc.Catalog.Ingredients(i18n.FromRequest(r)))
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Sid    string            `json:"sid"`
		Events []service.EventIn `json:"events"`
	}
	if !decode(w, r, 32<<10, &body) {
		return
	}
	if err := s.svc.Events.Track(r.Context(), body.Sid, body.Events); err != nil {
		if errors.Is(err, domain.ErrBadInput) {
			writeErr(w, 400, "bad batch")
			return
		}
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}
