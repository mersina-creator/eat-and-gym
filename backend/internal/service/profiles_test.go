package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"racion/internal/domain"
)

type memProfiles struct{ m map[string]domain.UserProfile }

func (r *memProfiles) Get(_ context.Context, id string) (domain.UserProfile, error) {
	p, ok := r.m[id]
	if !ok {
		return p, domain.ErrNotFound
	}
	return p, nil
}
func (r *memProfiles) Save(_ context.Context, id string, p domain.UserProfile) error {
	r.m[id] = p
	return nil
}

func TestProfilesSaveAndRecalc(t *testing.T) {
	s := NewProfiles(&memProfiles{m: map[string]domain.UserProfile{}})
	s.now = func() time.Time { return time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) }
	ctx := context.Background()

	if v, err := s.Get(ctx, "u1"); v != nil || err != nil {
		t.Fatalf("пустой профиль: %v %v", v, err)
	}
	p := domain.UserProfile{Sex: "m", BirthDate: "1996-01-01", HeightCm: 180, WeightKg: 80, Activity: "moderate", Goal: "maintain"}
	v, err := s.Save(ctx, "u1", p)
	if err != nil || v.Targets.Kcal != 2760 || v.Profile.Pace != "normal" {
		t.Fatalf("сохранение: %+v %v", v, err)
	}
	p.Goal = "lose"
	v, _ = s.Save(ctx, "u1", p)
	if v.Targets.Kcal >= 2760 {
		t.Fatalf("смена цели не пересчитала норму: %d", v.Targets.Kcal)
	}
	if k := s.KcalTarget(ctx, "u1"); k != float64(v.Targets.Kcal) {
		t.Fatalf("норма для планировщика %v", k)
	}

	p.HeightCm = 50
	_, err = s.Save(ctx, "u1", p)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || ve.Key != "profile.err.height" {
		t.Fatalf("ожидали ошибку роста, получили %v", err)
	}
}
