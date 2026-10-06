package nutrition

import (
	"testing"
	"time"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func born(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

func base() Profile {
	return Profile{Sex: Male, BirthDate: born(1996, 1, 1), HeightCm: 180, WeightKg: 80, Activity: "moderate", Goal: Maintain, Pace: Normal}
}

func TestAge(t *testing.T) {
	if a := Age(born(1996, 10, 6), now); a != 29 {
		t.Fatalf("день рождения завтра: 29, получили %d", a)
	}
	if a := Age(born(1996, 10, 5), now); a != 30 {
		t.Fatalf("день рождения сегодня: 30, получили %d", a)
	}
}

func TestMifflin(t *testing.T) {
	// 10×80 + 6,25×180 − 5×30 + 5 = 1780
	if r := Mifflin(Male, 80, 180, 30); r != 1780 {
		t.Fatalf("муж: 1780, получили %v", r)
	}
	// 10×60 + 6,25×165 − 5×30 − 161 = 1320,25
	if r := Mifflin(Female, 60, 165, 30); r != 1320.25 {
		t.Fatalf("жен: 1320,25, получили %v", r)
	}
}

func TestCalcMaintain(t *testing.T) {
	r := Calc(base(), now)
	if r.Method != "mifflin" || r.RMR != 1780 {
		t.Fatalf("RMR: %+v", r)
	}
	if r.TDEE != 2759 { // 1780 × 1,55
		t.Fatalf("TDEE 2759, получили %d", r.TDEE)
	}
	if r.Kcal != 2760 || r.ShiftPct != 0 {
		t.Fatalf("поддержание: %+v", r)
	}
	if r.ProteinG != 128 { // 1,6 × 80
		t.Fatalf("белок 128, получили %d", r.ProteinG)
	}
	sum := r.ProteinG*4 + r.CarbsG*4 + r.FatG*9
	if d := sum - r.Kcal; d < -10 || d > 10 {
		t.Fatalf("БЖУ не сходятся с калориями: %d против %d", sum, r.Kcal)
	}
}

func TestCalcLoseUsesCunninghamWithBodyFat(t *testing.T) {
	p := base()
	p.Goal, p.BodyFatPct = Lose, 20
	r := Calc(p, now)
	// безжировая 64 кг → 500 + 22×64 = 1908
	if r.Method != "cunningham" || r.RMR != 1908 || r.LeanMassKg != 64 {
		t.Fatalf("Каннингем: %+v", r)
	}
	if r.ShiftPct != -15 || r.WeeklyKg >= 0 {
		t.Fatalf("похудение: %+v", r)
	}
	if r.ProteinG != 160 { // 2,0 × 80
		t.Fatalf("белок 160, получили %d", r.ProteinG)
	}
}

func TestFloor(t *testing.T) {
	p := Profile{Sex: Female, BirthDate: born(1970, 1, 1), HeightCm: 152, WeightKg: 50, Activity: "sedentary", Goal: Lose, Pace: Fast}
	r := Calc(p, now)
	if !r.FloorApplied || r.Kcal < 1200 || r.Kcal < r.RMR {
		t.Fatalf("минимум не сработал: %+v", r)
	}
}

func TestRefWeightForHighBMI(t *testing.T) {
	p := base()
	p.WeightKg, p.Goal = 130, Lose // BMI ≈ 40
	r := Calc(p, now)
	if r.ProteinG != 162 { // 2,0 × (25 × 1,8²) = 162
		t.Fatalf("белок по расчётному весу 162, получили %d", r.ProteinG)
	}
}

func TestValidate(t *testing.T) {
	if k := Validate(base(), now); k != "" {
		t.Fatalf("корректный профиль: %s", k)
	}
	cases := map[string]func(*Profile){
		"profile.err.age":         func(p *Profile) { p.BirthDate = born(2010, 1, 1) },
		"profile.err.height":      func(p *Profile) { p.HeightCm = 90 },
		"profile.err.weight":      func(p *Profile) { p.WeightKg = 20 },
		"profile.err.bodyfat":     func(p *Profile) { p.BodyFatPct = 80 },
		"profile.err.activity":    func(p *Profile) { p.Activity = "x" },
		"profile.err.goal":        func(p *Profile) { p.Goal = "x" },
		"profile.err.underweight": func(p *Profile) { p.Goal, p.WeightKg = Lose, 55 },
	}
	for want, mut := range cases {
		p := base()
		mut(&p)
		if got := Validate(p, now); got != want {
			t.Errorf("%s: получили %q", want, got)
		}
	}
}
