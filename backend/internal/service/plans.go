package service

import (
	"context"
	"regexp"
	"strings"
	"time"

	"racion/internal/domain"
	"racion/internal/i18n"
	"racion/internal/planner"
)

var planIDRe = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)

// IsPlanID — похоже ли на uuid плана; всё остальное сразу «не найдено», без запроса в БД.
func IsPlanID(id string) bool { return planIDRe.MatchString(id) }

// Plans — сборка недели, замена блюд, отметки и свои товары в списке, кабинет.
type Plans struct {
	plans       PlanRepo
	checks      CheckRepo
	purchases   PurchaseRepo
	extras      ExtraRepo
	dislikes    DislikeRepo
	members     PlanMemberRepo
	recipes     *Recipes
	social      *Social
	family      *Family
	catalog     *planner.CatalogRef
	collections *Collections
	cart        CartLinker // корзина сети ссылкой (ВкусВилл); nil — не подключена
	profiles    *Profiles  // норма калорий из профиля (ФТ-01); nil — не подключён
}

// Create собирает план по ответам квиза. Нелюбимые рецепты берутся только из аккаунта,
// что бы ни прислал клиент; свои рецепты владельца подмешиваются в каталог.
func (p *Plans) Create(ctx context.Context, params planner.Params, lang i18n.Lang, user *domain.User) (planner.Plan, error) {
	params.Lang = string(lang)
	params.ExcludeRecipes = nil
	params.Favorites = nil
	var ownerID *string
	params.Liked, params.Meh, params.CollectionIDs = nil, nil, nil
	if user != nil {
		params.ExcludeRecipes, _ = p.dislikes.List(ctx, user.ID)
		params.Favorites = p.social.FavoriteIDs(ctx, user.ID)
		var banned []string
		params.Liked, params.Meh, banned = p.social.Taste(ctx, user.ID)
		params.ExcludeRecipes = append(params.ExcludeRecipes, banned...)
		ownerID = &user.ID
		// ФТ-01: своя норма не вписана в анкете — берём рассчитанную по профилю
		if params.KcalTarget == 0 && p.profiles != nil {
			params.KcalTarget = p.profiles.KcalTarget(ctx, user.ID)
		}
	}
	if params.Collection != "" && p.collections != nil {
		uid := ""
		if user != nil {
			uid = user.ID
		}
		params.CollectionIDs = p.collections.IDs(ctx, uid, params.Collection)
	}
	plan := p.recipes.CatalogFor(ctx, ownerID).Build(params)
	id, err := p.plans.Insert(ctx, plan, ownerID)
	if err != nil {
		return plan, err
	}
	plan.ID = id
	return plan, nil
}

// load читает план и возвращает каталог его владельца: свои рецепты владельца должны открываться
// и пересобираться у любого, кто смотрит план по ссылке.
type loaded struct {
	planner.Plan
	ownerID *string
}

func (p *Plans) load(ctx context.Context, id string) (loaded, *planner.Catalog, error) {
	if !IsPlanID(id) {
		return loaded{}, nil, domain.ErrNotFound
	}
	rec, err := p.plans.Get(ctx, id)
	if err != nil {
		return loaded{}, nil, err
	}
	plan := rec.Plan
	// Планы, собранные до появления стран и языков: Россия, русский.
	if plan.Country.Code == "" {
		plan.Country = planner.CountryOf(plan.Params.Country)
	}
	if plan.Lang == "" {
		plan.Lang, plan.Params.Lang = "ru", "ru"
	}
	return loaded{Plan: plan, ownerID: rec.OwnerID}, p.recipes.CatalogFor(ctx, rec.OwnerID), nil
}

// Get — план на языке запроса.
func (p *Plans) Get(ctx context.Context, id string, lang i18n.Lang) (planner.Plan, error) {
	l, cat, err := p.load(ctx, id)
	if err != nil {
		return l.Plan, err
	}
	return cat.Localize(l.Plan, lang), nil
}

// Original — план на языке, на котором его собрали. Бот пишет на языке недели, а не телефона:
// собирали по-русски — и список нужен по-русски.
func (p *Plans) Original(ctx context.Context, id string) (planner.Plan, i18n.Lang, error) {
	l, cat, err := p.load(ctx, id)
	if err != nil {
		return l.Plan, i18n.RU, err
	}
	lang, ok := i18n.Valid(l.Plan.Lang)
	if !ok {
		lang = i18n.RU
	}
	return cat.Localize(l.Plan, lang), lang, nil
}

// Swap меняет блюдо в ячейке и сохраняет план. Чужой план по ссылке менять нельзя: только смотреть.
func (p *Plans) Swap(ctx context.Context, id string, day int, slot string, lang i18n.Lang, viewer *domain.User) (planner.Plan, error) {
	l, cat, err := p.load(ctx, id)
	if err != nil {
		return l.Plan, err
	}
	if !p.canEdit(ctx, id, l.ownerID, viewer) {
		return l.Plan, domain.ErrForbidden
	}
	plan := l.Plan
	if viewer != nil {
		plan.Params.ExcludeRecipes, _ = p.dislikes.List(ctx, viewer.ID)
		plan.Params.Favorites = p.social.FavoriteIDs(ctx, viewer.ID)
		var banned []string
		plan.Params.Liked, plan.Params.Meh, banned = p.social.Taste(ctx, viewer.ID)
		plan.Params.ExcludeRecipes = append(plan.Params.ExcludeRecipes, banned...)
	}
	plan = cat.Localize(plan, lang)
	updated, err := cat.Swap(plan, day, slot)
	if err != nil {
		return plan, domain.Invalid(err.Error())
	}
	if err := p.plans.Save(ctx, updated); err != nil {
		return plan, err
	}
	return updated, nil
}

// SwapSide — другой гарнир к тому же блюду.
func (p *Plans) SwapSide(ctx context.Context, id string, day int, slot string, lang i18n.Lang, viewer *domain.User) (planner.Plan, error) {
	l, cat, err := p.load(ctx, id)
	if err != nil {
		return l.Plan, err
	}
	if !p.canEdit(ctx, id, l.ownerID, viewer) {
		return l.Plan, domain.ErrForbidden
	}
	plan := cat.Localize(l.Plan, lang)
	updated, err := cat.SwapSide(plan, day, slot)
	if err != nil {
		return plan, domain.Invalid(err.Error())
	}
	if err := p.plans.Save(ctx, updated); err != nil {
		return plan, err
	}
	return updated, nil
}

// CreateBasket — стол из блюд, которые человек выбрал сам. Отличается от события только тем, что
// состав задаёт он: дальше это обычный план со списком покупок и ценами.
func (p *Plans) CreateBasket(ctx context.Context, items []planner.BasketItem, params planner.Params, guests int, lang i18n.Lang, user *domain.User) (planner.Plan, error) {
	params.Lang = string(lang)
	params.ExcludeRecipes, params.Favorites, params.Liked, params.Meh, params.CollectionIDs = nil, nil, nil, nil, nil
	var ownerID *string
	if user != nil {
		ownerID = &user.ID
	}
	plan, err := p.recipes.CatalogFor(ctx, ownerID).BuildBasket(items, params, guests)
	if err != nil {
		return plan, domain.Invalid("basket.empty")
	}
	pid, err := p.plans.Insert(ctx, plan, ownerID)
	if err != nil {
		return plan, err
	}
	plan.ID = pid
	return plan, nil
}

// CreateOccasion — меню события на гостей вместо недели; сохраняется как обычный план.
func (p *Plans) CreateOccasion(ctx context.Context, id string, params planner.Params, guests int, lang i18n.Lang, user *domain.User) (planner.Plan, error) {
	o, ok := planner.OccasionByID(id)
	if !ok || o.Kind == "week" {
		return planner.Plan{}, domain.ErrNotFound
	}
	params.Lang = string(lang)
	params.ExcludeRecipes, params.Favorites, params.Liked, params.Meh, params.CollectionIDs = nil, nil, nil, nil, nil
	var ownerID *string
	if user != nil {
		params.ExcludeRecipes, _ = p.dislikes.List(ctx, user.ID)
		ownerID = &user.ID
	}
	plan, err := p.recipes.CatalogFor(ctx, ownerID).BuildOccasion(o, params, guests)
	if err != nil {
		return plan, domain.Invalid("occasion.empty")
	}
	pid, err := p.plans.Insert(ctx, plan, ownerID)
	if err != nil {
		return plan, err
	}
	plan.ID = pid
	return plan, nil
}

// edit — общая обёртка правок недели: право на правку, изменение, сохранение.
func (p *Plans) edit(ctx context.Context, id string, lang i18n.Lang, viewer *domain.User, fn func(cat *planner.Catalog, plan planner.Plan) (planner.Plan, error)) (planner.Plan, error) {
	l, cat, err := p.load(ctx, id)
	if err != nil {
		return l.Plan, err
	}
	if !p.canEdit(ctx, id, l.ownerID, viewer) {
		return l.Plan, domain.ErrForbidden
	}
	updated, err := fn(cat, cat.Localize(l.Plan, lang))
	if err != nil {
		return l.Plan, domain.Invalid(err.Error())
	}
	if err := p.plans.Save(ctx, updated); err != nil {
		return updated, err
	}
	return updated, nil
}

// SetSkip — день «не дома»: покупки и итоги без него.
func (p *Plans) SetSkip(ctx context.Context, id string, day int, skip bool, lang i18n.Lang, viewer *domain.User) (planner.Plan, error) {
	return p.edit(ctx, id, lang, viewer, func(cat *planner.Catalog, plan planner.Plan) (planner.Plan, error) {
		return cat.SetSkip(plan, day, skip)
	})
}

// Move — поменять местами блюда одного приёма в двух днях.
func (p *Plans) Move(ctx context.Context, id string, from, to int, slot string, lang i18n.Lang, viewer *domain.User) (planner.Plan, error) {
	return p.edit(ctx, id, lang, viewer, func(cat *planner.Catalog, plan planner.Plan) (planner.Plan, error) {
		return cat.Move(plan, from, to, slot)
	})
}

// Repeat — та же неделя на новые даты, новым планом пользователя.
func (p *Plans) Repeat(ctx context.Context, id, start string, lang i18n.Lang, user domain.User) (planner.Plan, error) {
	l, cat, err := p.load(ctx, id)
	if err != nil {
		return l.Plan, err
	}
	// без даты — ближайший понедельник, но не раньше следующей за этой неделей
	if start == "" {
		start = planner.NextMonday(time.Now()).Format("2006-01-02")
		if cur, err := time.Parse("2006-01-02", l.Plan.Params.StartDate); err == nil && start <= l.Plan.Params.StartDate {
			start = cur.AddDate(0, 0, 7).Format("2006-01-02")
		}
	}
	src := cat.Localize(l.Plan, lang)
	// Чужую неделю повторяют без имён: копия становится своей, и иначе повтор оказался бы способом
	// достать имена и возраст детей из любой ссылки.
	if !p.seesNames(ctx, id, l.ownerID, &user) {
		src.Anonymize()
	}
	plan, err := cat.Repeat(src, start)
	if err != nil {
		return plan, domain.Invalid(err.Error())
	}
	newID, err := p.plans.Insert(ctx, plan, &user.ID)
	if err != nil {
		return plan, err
	}
	plan.ID = newID
	return plan, nil
}

func (p *Plans) Mine(ctx context.Context, userID string) ([]domain.PlanSummary, error) {
	return p.plans.ByUser(ctx, userID)
}

func (p *Plans) Rename(ctx context.Context, id, userID, title string) error {
	title = strings.TrimSpace(title)
	if len(title) > 80 {
		title = title[:80]
	}
	return p.plans.Rename(ctx, id, userID, title)
}

func (p *Plans) Delete(ctx context.Context, id, userID string) error {
	return p.plans.Delete(ctx, id, userID)
}

// ── Отметки «куплено» ──────────────────────────────────────────────────────

// Checks — отмеченные позиции. Доступно и гостю по ссылке на план, чтобы список работал без входа.
func (p *Plans) Checks(ctx context.Context, planID string) ([]string, error) {
	if !IsPlanID(planID) {
		return nil, domain.ErrNotFound
	}
	out, err := p.checks.List(ctx, planID)
	if out == nil {
		out = []string{}
	}
	return out, err
}

// SetCheck ставит или снимает отметку. Для владельца плана покупка ещё и попадает в историю.
func (p *Plans) SetCheck(ctx context.Context, planID string, in domain.CheckInput, viewer *domain.User) error {
	if !IsPlanID(planID) {
		return domain.ErrNotFound
	}
	if in.ItemID == "" || len(in.ItemID) > 120 {
		return domain.ErrBadInput
	}
	ownerID, err := p.plans.OwnerID(ctx, planID)
	if err != nil {
		return err
	}
	if in.Checked {
		if err := p.checks.Set(ctx, planID, in.ItemID); err != nil {
			return err
		}
		if viewer != nil && ownerID != nil && *ownerID == viewer.ID {
			name := strings.TrimSpace(in.Name)
			if len(name) > 120 {
				name = name[:120]
			}
			_ = p.purchases.Add(ctx, viewer.ID, planID, in.ItemID, name, in.Qty, in.Cost)
		}
		return nil
	}
	_ = p.checks.Unset(ctx, planID, in.ItemID)
	if viewer != nil {
		_ = p.purchases.RemoveLatest(ctx, viewer.ID, planID, in.ItemID)
	}
	return nil
}

// ── Свои товары в списке ───────────────────────────────────────────────────

func (p *Plans) Extras(ctx context.Context, planID string) ([]domain.Extra, error) {
	if !IsPlanID(planID) {
		return nil, domain.ErrNotFound
	}
	return p.extras.List(ctx, planID)
}

func (p *Plans) AddExtra(ctx context.Context, planID string, e domain.Extra) (domain.Extra, error) {
	if !IsPlanID(planID) {
		return e, domain.ErrNotFound
	}
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" || len(e.Name) > 120 {
		return e, domain.Invalid("extras.empty")
	}
	if len(e.Qty) > 40 {
		e.Qty = e.Qty[:40]
	}
	if len(e.Note) > 200 {
		e.Note = e.Note[:200]
	}
	var due *time.Time
	if e.Due != nil && *e.Due != "" {
		if t, err := time.Parse("2006-01-02", *e.Due); err == nil {
			due = &t
		}
	}
	id, err := p.extras.Add(ctx, planID, e, due)
	if err != nil {
		return e, err
	}
	e.ID = id
	return e, nil
}

func (p *Plans) DeleteExtra(ctx context.Context, planID string, id int64) error {
	if !IsPlanID(planID) {
		return domain.ErrNotFound
	}
	return p.extras.Delete(ctx, planID, id)
}

// ── Семья ──────────────────────────────────────────────────────────────────

// canEdit — менять план может владелец и присоединившиеся; гостевой план (без владельца) — любой по ссылке.
func (p *Plans) canEdit(ctx context.Context, planID string, ownerID *string, viewer *domain.User) bool {
	if ownerID == nil {
		return true
	}
	if viewer == nil {
		return false
	}
	if *ownerID == viewer.ID {
		return true
	}
	if ok, _ := p.members.Is(ctx, planID, viewer.ID); ok {
		return true
	}
	return p.family != nil && p.family.Together(ctx, *ownerID, viewer.ID)
}

// CanSeeNames — имена едоков, детей и аккаунтов видит только своя семья: владелец, присоединившиеся
// и его домашние. Гостевой план без владельца имён не отдаёт никому: менять его может любой по ссылке,
// но ссылка уходит к чужим людям, а автор хранит имена у себя в браузере.
func (p *Plans) CanSeeNames(ctx context.Context, planID string, viewer *domain.User) bool {
	if viewer == nil || !IsPlanID(planID) {
		return false
	}
	ownerID, err := p.plans.OwnerID(ctx, planID)
	if err != nil {
		return false
	}
	return p.seesNames(ctx, planID, ownerID, viewer)
}

// seesNames — то же, когда владелец уже известен: план загружен и второй раз в базу идти незачем.
func (p *Plans) seesNames(ctx context.Context, planID string, ownerID *string, viewer *domain.User) bool {
	if viewer == nil || ownerID == nil {
		return false
	}
	return p.canEdit(ctx, planID, ownerID, viewer)
}

// Join добавляет пользователя в семью плана: неделя появляется в его кабинете, он может менять блюда.
func (p *Plans) Join(ctx context.Context, planID string, user domain.User) error {
	if !IsPlanID(planID) {
		return domain.ErrNotFound
	}
	ownerID, err := p.plans.OwnerID(ctx, planID)
	if err != nil {
		return err
	}
	if ownerID == nil {
		// ничейный план забирает первый вошедший
		return p.plans.Claim(ctx, planID, user.ID)
	}
	if *ownerID == user.ID {
		return nil
	}
	return p.members.Add(ctx, planID, user.ID)
}

// Family — имена участников плана для подписи на чеке (пусто, если никто не присоединялся).
func (p *Plans) Family(ctx context.Context, planID string) []string {
	if !IsPlanID(planID) {
		return nil
	}
	names, _ := p.members.Names(ctx, planID)
	if len(names) == 0 && p.family != nil {
		if owner, err := p.plans.OwnerID(ctx, planID); err == nil && owner != nil {
			names = p.family.AccountNames(ctx, *owner)
		}
	}
	return names
}
