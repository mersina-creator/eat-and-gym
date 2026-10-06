package journal

import "time"

// Потребление и адаптивная норма. Здесь живёт вся арифметика фазы 2: как из журнала еды
// и тренда веса получить фактический суточный расход и насколько сдвинуть норму.
//
// Главное правило пакета: день без данных и день с нулём — разные вещи. Первый в расчёт
// не входит вообще, второй входит нулём. Смешать их значит тихо сломать норму: пропуски
// утянут среднее вниз, формула решит, что человек недоедает, и задерёт калораж.

// Status дня в журнале.
const (
	StatusPlanned = "planned" // день закрыт, съедено по плану
	StatusPartial = "partial" // день закрыт с отклонениями
	StatusFasted  = "fasted"  // осознанный ноль: голодание, разгрузка
)

// DayIntake — съеденное за день. Пустой Status означает «нет данных»:
// такого дня в журнале нет, и в расчёты он не идёт.
type DayIntake struct {
	Date    time.Time `json:"date"`
	Status  string    `json:"status,omitempty"`
	Kcal    float64   `json:"kcal,omitempty"`
	Protein float64   `json:"protein,omitempty"`
	Fat     float64   `json:"fat,omitempty"`
	Carb    float64   `json:"carb,omitempty"`
	// Target и ProteinTarget — нормы, действовавшие в этот день. Хранятся снимком:
	// при смене цели прошлый график не должен перерисовываться под новую норму.
	Target        float64 `json:"target,omitempty"`
	ProteinTarget float64 `json:"proteinTarget,omitempty"`
}

// Counted отвечает, идёт ли день в расчёты. Голодание идёт — нулём.
func (d DayIntake) Counted() bool { return d.Status != "" }

// Intake — сводка потребления за окно: средние по БЖУ и попадание в норму.
// Для адаптивной нормы хватило бы одних калорий, но человек смотрит сюда каждый день,
// и ему нужно видеть всю четвёрку рядом с целью.
type Intake struct {
	DaysWithFood int     `json:"daysWithFood"`
	AvgKcal      float64 `json:"avgKcal,omitempty"`
	AvgProtein   float64 `json:"avgProtein,omitempty"`
	AvgFat       float64 `json:"avgFat,omitempty"`
	AvgCarb      float64 `json:"avgCarb,omitempty"`
	// AvgTarget — средняя норма дней окна (снимок на каждый день).
	AvgTarget float64 `json:"avgTarget,omitempty"`
	// Hit — доля дней, уложившихся в норму по калориям с допуском Tolerance.
	Hit float64 `json:"hit,omitempty"`
	// ProteinHit — доля дней, где добран белок. Белок единственный макрос
	// с жёстким требованием: на дефиците он защищает мышцы от распада.
	ProteinHit float64 `json:"proteinHit,omitempty"`
}

// Tolerance — допуск попадания в норму. ±10 % по калориям: точнее не считает
// ни один дневник, а требовать точности, которой нет, значит обесценить показатель.
const Tolerance = 0.10

// Sum считает средние по дням с данными внутри окна [from, to] включительно.
// Дни без статуса пропускаются: нулями они не становятся никогда.
func Sum(days []DayIntake, from, to time.Time) Intake {
	from, to = truncate(from), truncate(to)
	var in Intake
	var kcal, protein, fat, carb, target float64
	var withTarget, hit, pHit int
	for _, d := range days {
		day := truncate(d.Date)
		if day.Before(from) || day.After(to) || !d.Counted() {
			continue
		}
		in.DaysWithFood++
		kcal += d.Kcal
		protein += d.Protein
		fat += d.Fat
		carb += d.Carb
		if d.Target > 0 {
			withTarget++
			target += d.Target
			if abs(d.Kcal-d.Target) <= d.Target*Tolerance {
				hit++
			}
			if d.ProteinTarget > 0 && d.Protein >= d.ProteinTarget {
				pHit++
			}
		}
	}
	if in.DaysWithFood > 0 {
		n := float64(in.DaysWithFood)
		in.AvgKcal = round1(kcal / n)
		in.AvgProtein = round1(protein / n)
		in.AvgFat = round1(fat / n)
		in.AvgCarb = round1(carb / n)
	}
	if withTarget > 0 {
		in.AvgTarget = round1(target / float64(withTarget))
		in.Hit = round2(float64(hit) / float64(withTarget))
		in.ProteinHit = round2(float64(pHit) / float64(withTarget))
	}
	return in
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// KcalPerKg — энергия килограмма массы тела. 7700 ккал — общепринятая оценка для жировой ткани;
// на коротком окне часть изменения приходится на воду и гликоген, поэтому пересчёт
// ограничен шагом в 150 ккал (см. NextTarget).
const KcalPerKg = 7700.0

// FactTDEE — фактический суточный расход по журналу:
//
//	TDEE = средние ккал − Δвес по тренду × 7700 / дней
//
// Если вес падает, человек тратит больше, чем ест, и расход выше съеденного — отсюда минус.
// deltaKg — изменение тренда за окно (отрицательное при похудении), days — длина окна.
func FactTDEE(avgKcal, deltaKg float64, days int) (float64, bool) {
	if days < 7 || avgKcal <= 0 {
		return 0, false // окно меньше недели: вода и гликоген перебьют сигнал
	}
	return round1(avgKcal - deltaKg*KcalPerKg/float64(days)), true
}

// MaxStep — предел изменения нормы за пересчёт. Больше 150 ккал в неделю двигать нельзя:
// на коротком окне ошибка оценки сопоставима с самим дефицитом, и норма начнёт качаться.
const MaxStep = 150.0

// NextTarget — новая норма: фактический расход со сдвигом под цель, но не дальше MaxStep
// от текущей. shiftPct — поправка цели в процентах: −15 для похудения, +8 для набора, 0 для поддержания.
// floor — безопасный минимум из профиля (не ниже обмена в покое и не ниже 1500/1200 ккал).
func NextTarget(current, factTDEE, shiftPct, floor float64) float64 {
	want := factTDEE * (1 + shiftPct/100)
	if want > current+MaxStep {
		want = current + MaxStep
	}
	if want < current-MaxStep {
		want = current - MaxStep
	}
	if floor > 0 && want < floor {
		want = floor
	}
	return round1(want)
}

// CoverIntake дополняет покрытие по весу данными о днях с едой.
// Отдельная функция, а не поле в Cover: вес и еда приходят из разных таблиц.
func CoverIntake(c Coverage, in Intake) Coverage {
	c.DaysWithFood = in.DaysWithFood
	return c
}
