package journal

import (
	"math"
	"testing"
	"time"
)

func days(spec ...any) []DayIntake {
	var out []DayIntake
	for i := 0; i < len(spec); i += 3 {
		out = append(out, DayIntake{
			Date:   d(spec[i].(string)),
			Status: spec[i+1].(string),
			Kcal:   spec[i+2].(float64),
		})
	}
	return out
}

// Главный тест модуля: дни без записи не считаются нулём и в среднее не входят.
// Именно эта ошибка тихо ломает адаптивную норму, и по интерфейсу её не заметить.
func TestAvgSkipsMissingDays(t *testing.T) {
	// два дня по 2000 ккал и три дня без данных
	list := []DayIntake{
		{Date: d("2026-10-01"), Status: StatusPlanned, Kcal: 2000},
		{Date: d("2026-10-02")},
		{Date: d("2026-10-03")},
		{Date: d("2026-10-04")},
		{Date: d("2026-10-05"), Status: StatusPlanned, Kcal: 2000},
	}
	in := Sum(list, d("2026-10-01"), d("2026-10-05"))
	if in.DaysWithFood != 2 {
		t.Fatalf("дней с едой %d, ждём 2", in.DaysWithFood)
	}
	if !near(in.AvgKcal, 2000) {
		t.Errorf("среднее %v, ждём 2000 — пропуски не должны тянуть вниз", in.AvgKcal)
	}
}

// Осознанный ноль входит в расчёт нулём: голодание не должно выглядеть как забытый день.
func TestFastedCountsAsZero(t *testing.T) {
	in := Sum(days(
		"2026-10-01", StatusPlanned, 2000.0,
		"2026-10-02", StatusFasted, 0.0,
	), d("2026-10-01"), d("2026-10-02"))
	if in.DaysWithFood != 2 {
		t.Fatalf("дней с едой %d, ждём 2 — голодание считается", in.DaysWithFood)
	}
	if !near(in.AvgKcal, 1000) {
		t.Errorf("среднее %v, ждём 1000", in.AvgKcal)
	}
}

func TestSumIgnoresOutsideWindow(t *testing.T) {
	in := Sum(days(
		"2026-09-20", StatusPlanned, 5000.0,
		"2026-10-02", StatusPlanned, 2000.0,
	), d("2026-10-01"), d("2026-10-14"))
	if in.DaysWithFood != 1 || !near(in.AvgKcal, 2000) {
		t.Errorf("окно не соблюдено: %+v", in)
	}
}

func TestSumEmpty(t *testing.T) {
	in := Sum(nil, d("2026-10-01"), d("2026-10-14"))
	if in.DaysWithFood != 0 || in.AvgKcal != 0 {
		t.Errorf("пустой журнал: %+v", in)
	}
}

// При похудении фактический расход выше съеденного: вес падает, значит тратится больше.
func TestFactTDEEDeficit(t *testing.T) {
	// ел 2000 ккал, за 14 дней тренд упал на 1 кг
	got, ok := FactTDEE(2000, -1, 14)
	if !ok {
		t.Fatal("расчёт не выполнился")
	}
	want := 2000 + 7700.0/14 // 2550
	if math.Abs(got-want) > 0.2 {
		t.Errorf("расход %v, ждём %v", got, want)
	}
	if got <= 2000 {
		t.Error("при снижении веса расход обязан быть выше съеденного")
	}
}

func TestFactTDEEStableWeight(t *testing.T) {
	got, _ := FactTDEE(2300, 0, 14)
	if !near(got, 2300) {
		t.Errorf("при неизменном весе расход равен съеденному, получили %v", got)
	}
}

func TestFactTDEENeedsWeek(t *testing.T) {
	if _, ok := FactTDEE(2000, -1, 5); ok {
		t.Error("на окне меньше недели считать нельзя: вода и гликоген перебьют сигнал")
	}
}

// Шаг изменения нормы ограничен: иначе на коротком окне норма начнёт качаться.
func TestNextTargetStepLimited(t *testing.T) {
	// расход оказался сильно выше текущей нормы
	got := NextTarget(2000, 3200, 0, 0)
	if got > 2000+MaxStep+0.01 {
		t.Errorf("норма %v, шаг больше %v", got, MaxStep)
	}
	got = NextTarget(2000, 1000, 0, 0)
	if got < 2000-MaxStep-0.01 {
		t.Errorf("норма %v, шаг вниз больше %v", got, MaxStep)
	}
}

func TestNextTargetApplnesGoal(t *testing.T) {
	// расход 2100, цель похудение −15 % → 1785, в пределах шага
	got := NextTarget(1800, 2100, -15, 0)
	if !near(got, 1785) {
		t.Errorf("норма %v, ждём 1785", got)
	}
}

// Безопасный минимум из профиля перебивает расчёт.
func TestNextTargetRespectsFloor(t *testing.T) {
	got := NextTarget(1700, 1600, -20, 1600)
	if got < 1600 {
		t.Errorf("норма %v ниже минимума 1600", got)
	}
}

// Пересчёт требует обеих сторон уравнения: и веса, и еды.
func TestReadyNeedsFoodAndWeight(t *testing.T) {
	full := Coverage{Days: 14, WeighIns: 6, DaysWithFood: 12, FreshDay: 1}
	if ok, why := ReadyToAdapt(full); !ok {
		t.Errorf("должно быть готово, получили %v", why)
	}

	noFood := Coverage{Days: 14, WeighIns: 6, DaysWithFood: 3, FreshDay: 1}
	ok, why := ReadyToAdapt(noFood)
	if ok || len(why) != 1 || why[0] != "few_days" {
		t.Errorf("мало дней с едой: ждём few_days, получили %v %v", ok, why)
	}

	nothing := Coverage{Days: 14, FreshDay: -1} // как возвращает Cover для пустого журнала
	ok, why = ReadyToAdapt(nothing)
	if ok || len(why) != 3 {
		t.Errorf("пустой журнал: ждём три причины, получили %v %v", ok, why)
	}
}

func TestCoverIntakeMerges(t *testing.T) {
	c := CoverIntake(Coverage{Days: 14, WeighIns: 5, FreshDay: 0}, Intake{DaysWithFood: 11})
	if c.DaysWithFood != 11 || c.WeighIns != 5 {
		t.Errorf("слияние покрытий: %+v", c)
	}
}

// Сквозной случай: две недели журнала, вес снижается — считаем расход и новую норму.
func TestEndToEnd(t *testing.T) {
	var ms []Measurement
	var ds []DayIntake
	for i := 0; i < 14; i++ {
		day := d("2026-10-01").AddDate(0, 0, i)
		ms = append(ms, Measurement{Date: day, Kg: 100 - float64(i)*0.07})
		ds = append(ds, DayIntake{Date: day, Status: StatusPlanned, Kcal: 2200})
	}
	from, to := d("2026-10-01"), d("2026-10-14")

	pts := Trend(ms, from, to)
	in := Sum(ds, from, to)
	cov := CoverIntake(Cover(ms, from, to), in)

	if ok, why := ReadyToAdapt(cov); !ok {
		t.Fatalf("данных достаточно, но пересчёт закрыт: %v", why)
	}

	delta := pts[len(pts)-1].Trend - pts[0].Trend
	tdee, ok := FactTDEE(in.AvgKcal, delta, cov.Days)
	if !ok {
		t.Fatal("расход не посчитался")
	}
	if tdee <= in.AvgKcal {
		t.Errorf("вес снижается, расход %v должен быть выше съеденных %v", tdee, in.AvgKcal)
	}

	next := NextTarget(2200, tdee, -15, 1500)
	if math.Abs(next-2200) > MaxStep+0.01 {
		t.Errorf("норма сдвинулась на %v, предел %v", next-2200, MaxStep)
	}
}

var _ = time.Now

// БЖУ считаются все четыре, а не только калории с белком.
func TestSumAllMacros(t *testing.T) {
	in := Sum([]DayIntake{
		{Date: d("2026-10-01"), Status: StatusPlanned, Kcal: 2000, Protein: 150, Fat: 60, Carb: 200},
		{Date: d("2026-10-02"), Status: StatusPlanned, Kcal: 2400, Protein: 170, Fat: 80, Carb: 240},
	}, d("2026-10-01"), d("2026-10-02"))

	if !near(in.AvgKcal, 2200) {
		t.Errorf("ккал %v, ждём 2200", in.AvgKcal)
	}
	if !near(in.AvgProtein, 160) {
		t.Errorf("белок %v, ждём 160", in.AvgProtein)
	}
	if !near(in.AvgFat, 70) {
		t.Errorf("жиры %v, ждём 70", in.AvgFat)
	}
	if !near(in.AvgCarb, 220) {
		t.Errorf("углеводы %v, ждём 220", in.AvgCarb)
	}
}

// Попадание в норму: допуск ±10 %, дни без снимка нормы в долю не входят.
func TestHitRate(t *testing.T) {
	in := Sum([]DayIntake{
		{Date: d("2026-10-01"), Status: StatusPlanned, Kcal: 2000, Target: 2000}, // точно
		{Date: d("2026-10-02"), Status: StatusPlanned, Kcal: 2180, Target: 2000}, // +9 %, попал
		{Date: d("2026-10-03"), Status: StatusPlanned, Kcal: 2500, Target: 2000}, // +25 %, мимо
		{Date: d("2026-10-04"), Status: StatusPlanned, Kcal: 1900},               // нормы нет
	}, d("2026-10-01"), d("2026-10-04"))

	if in.DaysWithFood != 4 {
		t.Fatalf("дней с едой %d, ждём 4", in.DaysWithFood)
	}
	if !near(in.Hit, 0.67) {
		t.Errorf("попадание %v, ждём 0,67 — два из трёх дней с нормой", in.Hit)
	}
	if !near(in.AvgTarget, 2000) {
		t.Errorf("средняя норма %v, ждём 2000", in.AvgTarget)
	}
}

// Белок: добран или нет. На дефиците это главный макрос — он защищает мышцы.
func TestProteinHit(t *testing.T) {
	in := Sum([]DayIntake{
		{Date: d("2026-10-01"), Status: StatusPlanned, Kcal: 2000, Protein: 170, Target: 2000, ProteinTarget: 164},
		{Date: d("2026-10-02"), Status: StatusPlanned, Kcal: 2000, Protein: 120, Target: 2000, ProteinTarget: 164},
	}, d("2026-10-01"), d("2026-10-02"))

	if !near(in.ProteinHit, 0.5) {
		t.Errorf("белок добран в %v дней, ждём 0,5", in.ProteinHit)
	}
}

// Голодание не ломает долю попаданий: ноль при норме 2000 — это мимо, и так и должно считаться.
func TestFastedCountsAsMiss(t *testing.T) {
	in := Sum([]DayIntake{
		{Date: d("2026-10-01"), Status: StatusPlanned, Kcal: 2000, Target: 2000},
		{Date: d("2026-10-02"), Status: StatusFasted, Kcal: 0, Target: 2000},
	}, d("2026-10-01"), d("2026-10-02"))

	if !near(in.Hit, 0.5) {
		t.Errorf("попадание %v, ждём 0,5", in.Hit)
	}
	if !near(in.AvgKcal, 1000) {
		t.Errorf("среднее %v, ждём 1000", in.AvgKcal)
	}
}
