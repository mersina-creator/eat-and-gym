package journal

import (
	"math"
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func ms(pairs ...any) []Measurement {
	var out []Measurement
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, Measurement{Date: d(pairs[i].(string)), Kg: pairs[i+1].(float64)})
	}
	return out
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.051 }

// Первое значение тренда равно первому замеру, дальше сглаживание с шагом Alpha.
func TestTrendSmoothing(t *testing.T) {
	p := Trend(ms("2026-10-01", 100.0, "2026-10-02", 102.0, "2026-10-03", 101.0), d("2026-10-01"), d("2026-10-03"))
	if len(p) != 3 {
		t.Fatalf("дней %d, ждём 3", len(p))
	}
	if !near(p[0].Trend, 100) {
		t.Errorf("первый тренд %v, ждём 100", p[0].Trend)
	}
	// 100 + 0,1*(102-100) = 100,2
	if !near(p[1].Trend, 100.2) {
		t.Errorf("второй тренд %v, ждём 100,2", p[1].Trend)
	}
	// 100,2 + 0,1*(101-100,2) = 100,28 → 100,3
	if !near(p[2].Trend, 100.3) {
		t.Errorf("третий тренд %v, ждём 100,3", p[2].Trend)
	}
	// скачок в 2 кг за день сглажен до 0,2 — ради этого всё и затевалось
	if p[1].Trend-p[0].Trend > 0.5 {
		t.Errorf("суточный скачок не сглажен: %v", p[1].Trend-p[0].Trend)
	}
}

// Пропуск переносит значение тренда и не создаёт движения.
func TestTrendGaps(t *testing.T) {
	p := Trend(ms("2026-10-01", 100.0, "2026-10-05", 99.0), d("2026-10-01"), d("2026-10-05"))
	if len(p) != 5 {
		t.Fatalf("дней %d, ждём 5 — ряд должен быть непрерывным", len(p))
	}
	for i := 1; i <= 3; i++ {
		if !near(p[i].Trend, 100) {
			t.Errorf("день %d: тренд %v, ждём перенос 100", i, p[i].Trend)
		}
		if p[i].Weight != 0 {
			t.Errorf("день %d: вес %v, ждём пусто", i, p[i].Weight)
		}
	}
	if !near(p[4].Trend, 99.9) {
		t.Errorf("после пропуска тренд %v, ждём 99,9", p[4].Trend)
	}
}

// Правка задним числом меняет весь хвост тренда.
func TestTrendRecalcOnEdit(t *testing.T) {
	before := Trend(ms("2026-10-01", 100.0, "2026-10-02", 100.0, "2026-10-03", 100.0), d("2026-10-01"), d("2026-10-03"))
	after := Trend(ms("2026-10-01", 100.0, "2026-10-02", 90.0, "2026-10-03", 100.0), d("2026-10-01"), d("2026-10-03"))
	if near(before[2].Trend, after[2].Trend) {
		t.Errorf("правка за 2 октября не изменила хвост: %v и %v", before[2].Trend, after[2].Trend)
	}
}

// Два замера за одну дату: побеждает последний, лишней точки нет.
func TestTrendSameDayOverwrite(t *testing.T) {
	p := Trend(ms("2026-10-01", 100.0, "2026-10-01", 105.0), d("2026-10-01"), d("2026-10-01"))
	if len(p) != 1 {
		t.Fatalf("точек %d, ждём 1", len(p))
	}
	if !near(p[0].Weight, 105) {
		t.Errorf("вес %v, ждём 105 — последний за дату", p[0].Weight)
	}
}

// Замеры до начала окна участвуют в расчёте, но в вывод не попадают.
func TestTrendWindowKeepsHistory(t *testing.T) {
	all := ms("2026-10-01", 100.0, "2026-10-02", 100.0, "2026-10-10", 98.0)
	p := Trend(all, d("2026-10-09"), d("2026-10-10"))
	if len(p) != 2 {
		t.Fatalf("точек %d, ждём 2", len(p))
	}
	if p[0].Trend == 0 {
		t.Error("тренд в начале окна пуст: история до окна потеряна")
	}
}

func TestRatePerWeek(t *testing.T) {
	// ровное снижение на 0,1 кг тренда в день — это 0,7 кг в неделю
	var list []Measurement
	for i := 0; i < 15; i++ {
		list = append(list, Measurement{Date: d("2026-10-01").AddDate(0, 0, i), Kg: 100 - float64(i)})
	}
	p := Trend(list, d("2026-10-01"), d("2026-10-15"))
	rate, ok := RatePerWeek(p)
	if !ok {
		t.Fatal("скорость не посчиталась")
	}
	if rate >= 0 {
		t.Errorf("скорость %v, ждём отрицательную при снижении веса", rate)
	}
}

func TestRateNeedsFourPoints(t *testing.T) {
	p := Trend(ms("2026-10-01", 100.0, "2026-10-02", 99.0, "2026-10-03", 98.0), d("2026-10-01"), d("2026-10-03"))
	if _, ok := RatePerWeek(p); ok {
		t.Error("по трём замерам скорость считаться не должна: это экстраполяция шума")
	}
}

func TestCoverage(t *testing.T) {
	c := Cover(ms("2026-10-01", 100.0, "2026-10-03", 100.0, "2026-10-14", 99.0), d("2026-10-01"), d("2026-10-14"))
	if c.Days != 14 {
		t.Errorf("окно %d дней, ждём 14", c.Days)
	}
	if c.WeighIns != 3 {
		t.Errorf("замеров %d, ждём 3", c.WeighIns)
	}
	if c.FreshDay != 0 {
		t.Errorf("свежесть %d, ждём 0 — замер в последний день окна", c.FreshDay)
	}
}

// Замеры вне окна в покрытие не идут.
func TestCoverageIgnoresOutside(t *testing.T) {
	c := Cover(ms("2026-09-01", 100.0, "2026-10-05", 100.0), d("2026-10-01"), d("2026-10-14"))
	if c.WeighIns != 1 {
		t.Errorf("замеров %d, ждём 1", c.WeighIns)
	}
}

// Условия по весу. Полный набор условий, включая еду, проверяет TestReadyNeedsFoodAndWeight.
func TestReadyToAdapt(t *testing.T) {
	ok, why := ReadyToAdapt(Coverage{Days: 14, WeighIns: 6, DaysWithFood: 12, FreshDay: 1})
	if !ok || len(why) != 0 {
		t.Errorf("должно быть готово, получили %v %v", ok, why)
	}

	ok, why = ReadyToAdapt(Coverage{Days: 14, WeighIns: 2, DaysWithFood: 12, FreshDay: 1})
	if ok || len(why) != 1 || why[0] != "few_weighins" {
		t.Errorf("мало замеров: ждём few_weighins, получили %v %v", ok, why)
	}

	ok, why = ReadyToAdapt(Coverage{Days: 14, WeighIns: 6, DaysWithFood: 12, FreshDay: 9})
	if ok || len(why) != 1 || why[0] != "stale_weighin" {
		t.Errorf("старый замер: ждём stale_weighin, получили %v %v", ok, why)
	}

	if ok, _ := ReadyToAdapt(Coverage{Days: 14, WeighIns: 0, DaysWithFood: 0, FreshDay: -1}); ok {
		t.Error("без замеров пересчёт включаться не должен")
	}
}

// Пустой вход не должен ронять расчёт.
func TestEmpty(t *testing.T) {
	if p := Trend(nil, d("2026-10-01"), d("2026-10-14")); p != nil {
		t.Errorf("без замеров ждём nil, получили %d точек", len(p))
	}
	if c := Cover(nil, d("2026-10-01"), d("2026-10-14")); c.WeighIns != 0 || c.FreshDay != -1 {
		t.Errorf("пустое покрытие: %+v", c)
	}
}
