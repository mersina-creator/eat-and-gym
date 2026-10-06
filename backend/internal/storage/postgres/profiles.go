package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"racion/internal/domain"
)

type Profiles struct{ pool *pgxpool.Pool }

// Get — профиль аккаунта; domain.ErrNotFound, если его ещё не заполняли.
func (r *Profiles) Get(ctx context.Context, userID string) (domain.UserProfile, error) {
	var p domain.UserProfile
	var birth, upd time.Time
	var fat *float64
	err := r.pool.QueryRow(ctx, `SELECT sex, birth_date, height_cm::float8, weight_kg::float8, body_fat_pct::float8, activity, goal, pace, updated_at
		FROM user_profiles WHERE user_id = $1`, userID).
		Scan(&p.Sex, &birth, &p.HeightCm, &p.WeightKg, &fat, &p.Activity, &p.Goal, &p.Pace, &upd)
	if err != nil {
		return p, wrap("profiles.get", err)
	}
	p.BirthDate = birth.Format("2006-01-02")
	if fat != nil {
		p.BodyFatPct = *fat
	}
	p.UpdatedAt = upd.UTC().Format(time.RFC3339)
	return p, nil
}

// Save — вставка или замена профиля целиком.
func (r *Profiles) Save(ctx context.Context, userID string, p domain.UserProfile) error {
	var fat *float64
	if p.BodyFatPct > 0 {
		fat = &p.BodyFatPct
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO user_profiles (user_id, sex, birth_date, height_cm, weight_kg, body_fat_pct, activity, goal, pace, updated_at)
		VALUES ($1, $2, $3::date, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (user_id) DO UPDATE SET sex = EXCLUDED.sex, birth_date = EXCLUDED.birth_date, height_cm = EXCLUDED.height_cm,
			weight_kg = EXCLUDED.weight_kg, body_fat_pct = EXCLUDED.body_fat_pct, activity = EXCLUDED.activity,
			goal = EXCLUDED.goal, pace = EXCLUDED.pace, updated_at = now()`,
		userID, p.Sex, p.BirthDate, p.HeightCm, p.WeightKg, fat, p.Activity, p.Goal, p.Pace)
	return wrap("profiles.save", err)
}
