package service

import (
	"context"
	"errors"
	"math"
	"time"

	"racion/internal/domain"
	"racion/internal/nutrition"
)

// ProfileRepo — хранилище профиля для расчёта нормы (ФТ-01).
type ProfileRepo interface {
	Get(ctx context.Context, userID string) (domain.UserProfile, error)
	Save(ctx context.Context, userID string, p domain.UserProfile) error
}

// Profiles — профиль и норма калорий/БЖУ. Норма не хранится: считается из профиля при каждом чтении,
// поэтому любая правка профиля сразу меняет норму, а смена формул не требует миграции данных.
type Profiles struct {
	repo ProfileRepo
	now  func() time.Time
}

func NewProfiles(repo ProfileRepo) *Profiles { return &Profiles{repo: repo, now: time.Now} }

// ProfileView — профиль вместе с рассчитанной нормой.
type ProfileView struct {
	Profile domain.UserProfile `json:"profile"`
	Targets nutrition.Targets  `json:"targets"`
}

// Get возвращает профиль и норму; (nil, nil) — профиль ещё не заполнен.
func (s *Profiles) Get(ctx context.Context, userID string) (*ProfileView, error) {
	if s == nil || s.repo == nil { // тесты и стенды без хранилища профиля
		return nil, nil
	}
	p, err := s.repo.Get(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	in, key := s.parse(p)
	if key != "" { // старая запись, которая не проходит текущие правила: показываем профиль без нормы
		return &ProfileView{Profile: p}, nil
	}
	return &ProfileView{Profile: p, Targets: nutrition.Calc(in, s.now())}, nil
}

// Save проверяет профиль, сохраняет и возвращает новую норму.
func (s *Profiles) Save(ctx context.Context, userID string, p domain.UserProfile) (*ProfileView, error) {
	p.HeightCm = math.Round(p.HeightCm*10) / 10
	p.WeightKg = math.Round(p.WeightKg*10) / 10
	p.BodyFatPct = math.Round(p.BodyFatPct*10) / 10
	if p.Pace == "" {
		p.Pace = string(nutrition.Normal)
	}
	in, key := s.parse(p)
	if key != "" {
		return nil, domain.Invalid(key)
	}
	if err := s.repo.Save(ctx, userID, p); err != nil {
		return nil, err
	}
	p.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	return &ProfileView{Profile: p, Targets: nutrition.Calc(in, s.now())}, nil
}

// KcalTarget — дневная норма для планировщика; 0 — профиля нет или он неполный.
func (s *Profiles) KcalTarget(ctx context.Context, userID string) float64 {
	v, err := s.Get(ctx, userID)
	if err != nil || v == nil {
		return 0
	}
	return float64(v.Targets.Kcal)
}

func (s *Profiles) parse(p domain.UserProfile) (nutrition.Profile, string) {
	birth, err := time.Parse("2006-01-02", p.BirthDate)
	if err != nil {
		return nutrition.Profile{}, "profile.err.birth"
	}
	in := nutrition.Profile{
		Sex: nutrition.Sex(p.Sex), BirthDate: birth, HeightCm: p.HeightCm, WeightKg: p.WeightKg,
		BodyFatPct: p.BodyFatPct, Activity: p.Activity, Goal: nutrition.Goal(p.Goal), Pace: nutrition.Pace(p.Pace),
	}
	return in, nutrition.Validate(in, s.now())
}
