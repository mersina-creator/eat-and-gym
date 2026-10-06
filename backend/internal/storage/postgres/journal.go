package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"racion/internal/domain"
)

// Журнал веса: одно взвешивание в день на пользователя. Ключ (user_id, date),
// повторная запись за ту же дату перезаписывает значение.
type Journal struct{ pool *pgxpool.Pool }

func NewJournal(pool *pgxpool.Pool) *Journal { return &Journal{pool: pool} }

// SetWeight записывает или перезаписывает замер за дату.
func (r *Journal) SetWeight(ctx context.Context, userID string, date time.Time, kg float64, note string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO weight_log (user_id, date, weight_kg, note) VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, date) DO UPDATE SET weight_kg = EXCLUDED.weight_kg, note = EXCLUDED.note, created_at = now()`,
		userID, date, kg, note)
	return wrap("journal.setWeight", err)
}

// DeleteWeight убирает замер за дату. Второй результат — был ли он там.
func (r *Journal) DeleteWeight(ctx context.Context, userID string, date time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM weight_log WHERE user_id = $1 AND date = $2`, userID, date)
	if err != nil {
		return false, wrap("journal.deleteWeight", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Weights отдаёт замеры с даты from включительно, по возрастанию даты.
// Нулевое from — вся история: она нужна, чтобы тренд в начале окна не стартовал с пустого места.
func (r *Journal) Weights(ctx context.Context, userID string, from time.Time) ([]domain.WeightEntry, error) {
	rows, err := r.pool.Query(ctx, `SELECT date, weight_kg, note FROM weight_log
		WHERE user_id = $1 AND ($2::date IS NULL OR date >= $2) ORDER BY date`, userID, nullDate(from))
	if err != nil {
		return nil, wrap("journal.weights", err)
	}
	defer rows.Close()
	var out []domain.WeightEntry
	for rows.Next() {
		var e domain.WeightEntry
		if err := rows.Scan(&e.Date, &e.Kg, &e.Note); err != nil {
			return nil, wrap("journal.weights", err)
		}
		out = append(out, e)
	}
	return out, wrap("journal.weights", rows.Err())
}

// LastWeight — последний по дате замер. Второй результат false, если журнал пуст:
// по нему экран подставляет подсказку «в прошлый раз было столько-то».
func (r *Journal) LastWeight(ctx context.Context, userID string) (domain.WeightEntry, bool, error) {
	var e domain.WeightEntry
	err := r.pool.QueryRow(ctx, `SELECT date, weight_kg, note FROM weight_log
		WHERE user_id = $1 ORDER BY date DESC LIMIT 1`, userID).Scan(&e.Date, &e.Kg, &e.Note)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return e, false, nil
		}
		return e, false, wrap("journal.lastWeight", err)
	}
	return e, true, nil
}

func nullDate(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
