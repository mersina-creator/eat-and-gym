// Package service — сценарии приложения, не зависящие от транспорта. HTTP, очереди и пакетные задания
// вызывают одни и те же методы: на входе ctx и простые аргументы, на выходе доменные значения и ошибки из domain.
package service

import (
	"strings"
	"go.uber.org/zap"
	"context"
	"encoding/json"
	"time"

	"racion/internal/domain"
	"racion/internal/planner"
)

// ── Порты хранилища ────────────────────────────────────────────────────────
// Интерфейсы объявлены на стороне потребителя: postgres их реализует, тесты подменяют.

type UserRepo interface {
	Create(ctx context.Context, email, passwordHash, name string) (domain.User, error)
	SetPassword(ctx context.Context, id, passwordHash string) error
	ByEmail(ctx context.Context, email string) (domain.User, string, error)
	SetName(ctx context.Context, id, name string) error
	SetNick(ctx context.Context, id, nick string) error
	SetAvatar(ctx context.Context, id, url string) error
	SetRole(ctx context.Context, id, role string) error
	NickOf(ctx context.Context, id string) (string, error)
	SetDefaults(ctx context.Context, id string, defaults json.RawMessage) error
	ByOAuth(ctx context.Context, provider, providerID string) (domain.User, error)
	LinkOAuth(ctx context.Context, provider, providerID, userID, email string) error
	OAuthProviders(ctx context.Context, userID string) ([]string, error)
	UnlinkOAuth(ctx context.Context, userID, provider string) (bool, error)
}

// ResetRepo — одноразовые токены восстановления пароля
type ResetRepo interface {
	Create(ctx context.Context, tokenHash, userID string, expires time.Time) error
	Take(ctx context.Context, tokenHash string) (string, error)
}

// Mailer шлёт письма (восстановление пароля)
type Mailer interface {
	Send(to, subject, text string) error
}

type SessionRepo interface {
	Create(ctx context.Context, token, userID string, expires time.Time) error
	UserByToken(ctx context.Context, token string) (domain.User, error)
	Delete(ctx context.Context, token string) error
}

type PlanRepo interface {
	Insert(ctx context.Context, plan planner.Plan, ownerID *string) (string, error)
	Get(ctx context.Context, id string) (domain.PlanRecord, error)
	OwnerID(ctx context.Context, id string) (*string, error)
	Save(ctx context.Context, plan planner.Plan) error
	Claim(ctx context.Context, id, userID string) error
	Rename(ctx context.Context, id, userID, title string) error
	Delete(ctx context.Context, id, userID string) error
	ByUser(ctx context.Context, userID string) ([]domain.PlanSummary, error)
}

type DislikeRepo interface {
	List(ctx context.Context, userID string) ([]string, error)
	Add(ctx context.Context, userID, recipeID string) error
	Remove(ctx context.Context, userID, recipeID string) error
}

type CheckRepo interface {
	List(ctx context.Context, planID string) ([]string, error)
	Set(ctx context.Context, planID, itemID string) error
	Unset(ctx context.Context, planID, itemID string) error
}

type PurchaseRepo interface {
	Add(ctx context.Context, userID, planID, itemID, name, qty string, cost float64) error
	RemoveLatest(ctx context.Context, userID, planID, itemID string) error
	Recent(ctx context.Context, userID string, days int) ([]domain.Purchase, error)
}

type ExtraRepo interface {
	List(ctx context.Context, planID string) ([]domain.Extra, error)
	Add(ctx context.Context, planID string, e domain.Extra, due *time.Time) (int64, error)
	Delete(ctx context.Context, planID string, id int64) error
}

type UserRecipeRepo interface {
	ByUser(ctx context.Context, userID string) ([]planner.Recipe, error)
	Get(ctx context.Context, id string) (planner.Recipe, error)
	AddView(ctx context.Context, id string) error
	Count(ctx context.Context, userID string) (int, error)
	Insert(ctx context.Context, userID string, rc planner.Recipe) error
	Update(ctx context.Context, userID string, rc planner.Recipe) error
	Delete(ctx context.Context, userID, id string) error
	SetPublic(ctx context.Context, userID, id string, public bool) error
}

type PlanMemberRepo interface {
	Add(ctx context.Context, planID, userID string) error
	Is(ctx context.Context, planID, userID string) (bool, error)
	Names(ctx context.Context, planID string) ([]string, error)
}

type EventRepo interface {
	AddBatch(ctx context.Context, sid string, events []domain.Event) error
}

// Repos — всё хранилище одним значением, чтобы собирать сервисы в одну строку.
type Repos struct {
	Users       UserRepo
	Resets      ResetRepo
	Sessions    SessionRepo
	Plans       PlanRepo
	Dislikes    DislikeRepo
	Checks      CheckRepo
	Purchases   PurchaseRepo
	Extras      ExtraRepo
	UserRecipes UserRecipeRepo
	Events      EventRepo
	PlanMembers PlanMemberRepo
	Admin       AdminRepo
	Push        PushRepo
	Settings    SettingsRepo
	Social      SocialRepo
	Households  HouseholdRepo
	Collections CollectionRepo
	Partners    PartnerRepo
	Offers      OfferRepo
	APIKeys     APIKeyRepo
	Profiles    ProfileRepo
	Journal     JournalRepo
}

// Services — набор сценариев; транспорт получает его целиком.
type Services struct {
	Accounts     *Accounts
	Plans        *Plans
	Recipes      *Recipes
	Catalog      *Catalog
	Events       *Events
	Notify       *Notifications
	Social       *Social
	Family       *Family
	AI           *Assistant // nil-безопасен: без ключа отвечает «выключен»
	Admin        *Admin     // nil-безопасен: без ADMIN_EMAILS админов нет
	Media        *Media     // фото: nil-безопасен, без S3 отвечает «выключено»
	Moderation   *Moderation
	Translations *Translations
	PlanChat     *PlanChat
	CatalogAdmin *CatalogAdmin
	Subs         *Substitutes
	Collections  *Collections
	Partners     *Partners
	Offers       *Offers
	Ads          *Ads
	IndexNow     *IndexNow
	APIKeys      *APIKeys
	Profiles     *Profiles // ФТ-01: профиль и норма калорий/БЖУ
	Journal      *Journal  // ФТ-05: журнал веса
	Bots         *Bots // nil-безопасен: без токенов ботов кнопок «список в мессенджере» нет
}

// New собирает сервисы; subscriber и baseURL нужны push-уведомлениям (VAPID и ссылки в них).
func New(repos Repos, catalog *planner.CatalogRef, subscriber, baseURL string) *Services {
	recipes := &Recipes{repo: repos.UserRecipes, users: repos.Users, catalog: catalog}
	social := &Social{repo: repos.Social, recipes: recipes}
	family := &Family{repo: repos.Households}
	profiles := NewProfiles(repos.Profiles)
	return &Services{
		Accounts:    &Accounts{users: repos.Users, resets: repos.Resets, sessions: repos.Sessions, plans: repos.Plans, dislikes: repos.Dislikes, purchases: repos.Purchases, catalog: catalog, baseURL: strings.TrimRight(baseURL, "/")},
		Plans:       &Plans{plans: repos.Plans, checks: repos.Checks, purchases: repos.Purchases, extras: repos.Extras, dislikes: repos.Dislikes, members: repos.PlanMembers, recipes: recipes, social: social, family: family, catalog: catalog, collections: NewCollections(repos.Collections, recipes), profiles: profiles},
		Recipes:     recipes,
		Catalog:     &Catalog{catalog: catalog},
		Events:      &Events{repo: repos.Events},
		Notify:      NewNotifications(repos.Push, repos.Settings, subscriber, baseURL),
		Social:      social,
		Family:      family,
		Collections: NewCollections(repos.Collections, recipes),
		Partners:    NewPartners(repos.Partners),
		Offers:      NewOffers(repos.Offers),
		Ads:         NewAds(repos.Settings),
		IndexNow:    NewIndexNow(repos.Settings, baseURL, zap.L().Named("indexnow")),
		APIKeys:     NewAPIKeys(repos.APIKeys),
		AI:          NewAssistant(nil),
		Profiles:    profiles,
		Journal:     NewJournal(repos.Journal),
	}
}
