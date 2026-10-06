// Package postgres — репозитории поверх pgx. Только SQL и маппинг строк; правил здесь нет.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"racion/internal/domain"
)

// Store — все репозитории на одном пуле.
type Store struct {
	Users          *Users
	Resets         *Resets
	Sessions       *Sessions
	Plans          *Plans
	Dislikes       *Dislikes
	Checks         *Checks
	Purchases      *Purchases
	Extras         *Extras
	UserRecipes    *UserRecipes
	Events         *Events
	PlanMembers    *PlanMembers
	Push           *Push
	Messenger      *Messenger
	StorePrices    *StorePrices
	APIUsage       *APIUsage
	Settings       *Settings
	Social         *Social
	Households     *Households
	Admin          *Admin
	CatalogRecipes *CatalogRecipes
	Translations   *Translations
	Partners       *Partners
	Offers         *Offers
	APIKeys        *APIKeys
	Collections    *Collections
	Profiles       *Profiles
	Journal        *Journal
	pool           *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		Users: &Users{pool}, Resets: &Resets{pool}, Sessions: &Sessions{pool}, Plans: &Plans{pool}, Dislikes: &Dislikes{pool},
		Checks: &Checks{pool}, Purchases: &Purchases{pool}, Extras: &Extras{pool}, UserRecipes: &UserRecipes{pool}, Events: &Events{pool},
		PlanMembers: &PlanMembers{pool}, Push: &Push{pool}, Messenger: &Messenger{pool}, StorePrices: &StorePrices{pool}, APIUsage: &APIUsage{pool}, Settings: &Settings{pool}, Social: &Social{pool}, Households: &Households{pool}, Admin: &Admin{pool}, CatalogRecipes: &CatalogRecipes{pool}, Collections: &Collections{pool}, Translations: &Translations{pool}, Partners: &Partners{pool}, Offers: &Offers{pool}, APIKeys: &APIKeys{pool}, Profiles: &Profiles{pool}, Journal: &Journal{pool},
		pool: pool,
	}
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Cleanup удаляет просроченные сессии и события старше keepEvents; возвращает число строк.
func (s *Store) Cleanup(ctx context.Context, keepEvents time.Duration) (int64, error) {
	var total int64
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	if err != nil {
		return 0, wrap("cleanup.sessions", err)
	}
	total += tag.RowsAffected()
	tag, err = s.pool.Exec(ctx, `DELETE FROM messenger_links WHERE expires_at < now()`)
	if err != nil {
		return total, wrap("cleanup.links", err)
	}
	total += tag.RowsAffected()
	tag, err = s.pool.Exec(ctx, `DELETE FROM api_usage WHERE bucket < now() - interval '2 days'`)
	if err != nil {
		return total, wrap("cleanup.api_usage", err)
	}
	total += tag.RowsAffected()
	tag, err = s.pool.Exec(ctx, `DELETE FROM events WHERE ts < now() - make_interval(days => $1)`, int(keepEvents.Hours()/24))
	if err != nil {
		return total, wrap("cleanup.events", err)
	}
	return total + tag.RowsAffected(), nil
}

// wrap переводит ошибки pgx в доменные: нет строки → ErrNotFound, уникальность → ErrConflict.
func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505":
			return domain.ErrConflict
		case "23503": // внешний ключ: ссылка на несуществующий план
			return domain.ErrNotFound
		case "22P02": // не uuid в параметре: такого плана нет
			return domain.ErrNotFound
		}
	}
	return domain.Internal(op, err)
}

func scanStrings(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
