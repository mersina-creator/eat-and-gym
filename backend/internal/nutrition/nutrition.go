// Package nutrition — расчёт дневной нормы калорий и БЖУ по профилю. Чистые функции без БД и HTTP:
// всё, что здесь считается, проверяется тестами, а ИИ получает уже готовые цифры.
//
// Порядок расчёта:
//  1. Расход в покое (RMR): Каннингем по безжировой массе, если известен % жира; иначе Миффлин — Сан Жеор.
//  2. Суточный расход (TDEE) = RMR × коэффициент активности.
//  3. Калории = TDEE × (1 + поправка цели и темпа), но не ниже безопасного минимума.
//  4. Белок по весу (для BMI > 30 — по весу при BMI 25), жиры не ниже 20 % калорий, углеводы — остаток.
package nutrition

import (
	"math"
	"time"
)

type Sex string

const (
	Male   Sex = "m"
	Female Sex = "f"
)

type Goal string

const (
	Lose     Goal = "lose"
	Recomp   Goal = "recomp"
	Maintain Goal = "maintain"
	Gain     Goal = "gain"
)

type Pace string

const (
	Slow   Pace = "slow"
	Normal Pace = "normal"
	Fast   Pace = "fast"
)

// Activity — уровень активности; коэффициенты — стандартная шкала к RMR.
var Activity = map[string]float64{
	"sedentary": 1.2,   // сидячая работа, без тренировок
	"light":     1.375, // 1–3 тренировки в неделю
	"moderate":  1.55,  // 3–5 тренировок
	"high":      1.725, // 6–7 тренировок
	"extreme":   1.9,   // тяжёлый физический труд + тренировки
}

// goalShift — поправка к TDEE по цели и темпу (доля). Для похудения и набора темп меняет величину.
var goalShift = map[Goal]map[Pace]float64{
	Lose:     {Slow: -0.10, Normal: -0.15, Fast: -0.20},
	Recomp:   {Slow: -0.05, Normal: -0.05, Fast: -0.10},
	Maintain: {Slow: 0, Normal: 0, Fast: 0},
	Gain:     {Slow: 0.05, Normal: 0.08, Fast: 0.10},
}

// proteinPerKg — белок, г на кг расчётного веса. 1,6 г/кг — порог из мета-анализа Morton 2018,
// на дефиците и при рекомпозиции выше, чтобы сохранить мышцы; верх — 2,2 г/кг.
var proteinPerKg = map[Goal]float64{Lose: 2.0, Recomp: 2.0, Maintain: 1.6, Gain: 1.8}

const (
	fatPerKg      = 0.8  // г жира на кг расчётного веса
	fatMinShare   = 0.20 // жиры не ниже 20 % калорий
	kcalProtein   = 4.0
	kcalCarbs     = 4.0
	kcalFat       = 9.0
	kcalPerKgMass = 7700.0 // усреднённая энергия килограмма массы тела, для оценки темпа
)

// Profile — входные данные расчёта.
type Profile struct {
	Sex        Sex
	BirthDate  time.Time
	HeightCm   float64
	WeightKg   float64
	BodyFatPct float64 // 0 — неизвестен
	Activity   string
	Goal       Goal
	Pace       Pace
}

// Targets — результат расчёта: всё, что показываем человеку и отдаём планировщику.
type Targets struct {
	Method       string   `json:"method"` // mifflin | cunningham
	Age          int      `json:"age"`
	BMI          float64  `json:"bmi"`
	LeanMassKg   float64  `json:"leanMassKg,omitempty"`
	RMR          int      `json:"rmr"`
	TDEE         int      `json:"tdee"`
	Kcal         int      `json:"kcal"`
	ProteinG     int      `json:"proteinG"`
	FatG         int      `json:"fatG"`
	CarbsG       int      `json:"carbsG"`
	ShiftPct     int      `json:"shiftPct"`        // поправка цели к TDEE, %
	WeeklyKg     float64  `json:"weeklyKg"`        // ожидаемое изменение веса в неделю по расчёту
	FloorApplied bool     `json:"floorApplied"`    // калории подняты до безопасного минимума
	Notes        []string `json:"notes,omitempty"` // ключи i18n с пояснениями
}

// Age — полных лет на дату now.
func Age(birth, now time.Time) int {
	a := now.Year() - birth.Year()
	if now.Month() < birth.Month() || (now.Month() == birth.Month() && now.Day() < birth.Day()) {
		a--
	}
	return a
}

// Mifflin — RMR по формуле Миффлина — Сан Жеора (1990).
func Mifflin(sex Sex, weightKg, heightCm float64, age int) float64 {
	s := 5.0
	if sex == Female {
		s = -161
	}
	return 10*weightKg + 6.25*heightCm - 5*float64(age) + s
}

// Cunningham — RMR по безжировой массе (1980): точнее для тренирующихся, когда известен % жира.
func Cunningham(leanKg float64) float64 { return 500 + 22*leanKg }

// floorKcal — безопасный минимум: не ниже расхода в покое и не ниже 1200 (ж) / 1500 (м) ккал.
func floorKcal(sex Sex, rmr float64) float64 {
	base := 1500.0
	if sex == Female {
		base = 1200
	}
	return math.Max(base, rmr)
}

// Calc считает норму. Профиль должен быть уже проверен Validate.
func Calc(p Profile, now time.Time) Targets {
	t := Targets{Age: Age(p.BirthDate, now)}
	h := p.HeightCm / 100
	t.BMI = round1(p.WeightKg / (h * h))

	var rmr float64
	if p.BodyFatPct > 0 {
		lean := p.WeightKg * (1 - p.BodyFatPct/100)
		t.LeanMassKg = round1(lean)
		rmr = Cunningham(lean)
		t.Method = "cunningham"
	} else {
		rmr = Mifflin(p.Sex, p.WeightKg, p.HeightCm, t.Age)
		t.Method = "mifflin"
	}
	tdee := rmr * Activity[p.Activity]

	shift := goalShift[p.Goal][p.Pace]
	kcal := tdee * (1 + shift)
	if floor := floorKcal(p.Sex, rmr); kcal < floor && p.Goal != Gain {
		kcal = floor
		t.FloorApplied = true
		t.Notes = append(t.Notes, "profile.note.floor")
	}

	// Расчётный вес для белка и жиров: при BMI > 30 — вес при BMI 25, иначе белок раздувается за счёт жира.
	refW := p.WeightKg
	if t.BMI > 30 {
		refW = 25 * h * h
		t.Notes = append(t.Notes, "profile.note.refweight")
	}
	protein := proteinPerKg[p.Goal] * refW
	fat := math.Max(fatPerKg*refW, fatMinShare*kcal/kcalFat)
	carbs := (kcal - protein*kcalProtein - fat*kcalFat) / kcalCarbs
	if carbs < 0 { // очень маленькие калории: жиры опускаем до минимума доли, остаток — углеводы
		fat = fatMinShare * kcal / kcalFat
		carbs = math.Max(0, (kcal-protein*kcalProtein-fat*kcalFat)/kcalCarbs)
	}

	t.RMR = int(math.Round(rmr))
	t.TDEE = int(math.Round(tdee))
	t.Kcal = roundTo(kcal, 10)
	t.ProteinG = int(math.Round(protein))
	t.FatG = int(math.Round(fat))
	t.CarbsG = int(math.Round(carbs))
	t.ShiftPct = int(math.Round(shift * 100))
	t.WeeklyKg = round2((float64(t.Kcal) - tdee) * 7 / kcalPerKgMass)
	return t
}

// Validate проверяет профиль; возвращает ключ i18n ошибки или "".
func Validate(p Profile, now time.Time) string {
	switch {
	case p.Sex != Male && p.Sex != Female:
		return "profile.err.sex"
	case p.BirthDate.IsZero():
		return "profile.err.birth"
	}
	age := Age(p.BirthDate, now)
	h := p.HeightCm / 100
	switch {
	case age < 18 || age > 100:
		return "profile.err.age" // расчёты для взрослых: у подростков другие нормы
	case p.HeightCm < 120 || p.HeightCm > 230:
		return "profile.err.height"
	case p.WeightKg < 35 || p.WeightKg > 300:
		return "profile.err.weight"
	case p.BodyFatPct != 0 && (p.BodyFatPct < 3 || p.BodyFatPct > 60):
		return "profile.err.bodyfat"
	case Activity[p.Activity] == 0:
		return "profile.err.activity"
	case goalShift[p.Goal] == nil:
		return "profile.err.goal"
	}
	if _, ok := goalShift[p.Goal][p.Pace]; !ok {
		return "profile.err.pace"
	}
	if (p.Goal == Lose || p.Goal == Recomp) && p.WeightKg/(h*h) < 18.5 {
		return "profile.err.underweight" // дефицит при недостатке веса не считаем
	}
	return ""
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }
func round2(x float64) float64 { return math.Round(x*100) / 100 }
func roundTo(x float64, step int) int {
	return int(math.Round(x/float64(step))) * step
}
