// Package journal — расчёты журнала веса и питания: тренд, скорость, покрытие данными.
// Пакет чистый: на входе срезы значений, на выходе числа. Ни базы, ни HTTP, ни времени «сейчас» —
// поэтому его целиком закрывают тесты, а фаза 2 (адаптивная норма) опирается на него без оговорок.
package journal

import (
	"math"
	"sort"
	"time"
)

// Alpha — коэффициент экспоненциального сглаживания веса.
// 0,1 даёт период полуспада около недели: суточные качели на 1–2 кг от воды и соли гасятся,
// а реальное снижение видно через 3–4 дня. Меньше — линия отстаёт, больше — шумит.
const Alpha = 0.1

// Measurement — одно взвешивание. Date — календарная дата, без времени.
type Measurement struct {
	Date time.Time
	Kg   float64
}

// Point — день на графике. Weight нулевой, если в этот день не взвешивались;
// Trend заполнен всегда, начиная с первого замера, и тянется через пропуски.
type Point struct {
	Date   time.Time `json:"date"`
	Weight float64   `json:"weight,omitempty"`
	Trend  float64   `json:"trend,omitempty"`
}

// Trend разворачивает замеры в непрерывный ряд по календарным дням и сглаживает его.
//
// Ключевое правило: пропущенный день переносит предыдущее значение тренда без обновления.
// Так дыра в неделю не ломает кривую и не создаёт ложного движения — в отличие от сглаживания
// по номерам записей, где три пропуска выглядят как три дня резкого снижения.
//
// from и to задают окно вывода включительно. Замеры раньше from участвуют в расчёте
// (иначе тренд в начале окна стартовал бы с нуля), но в результат не попадают.
func Trend(ms []Measurement, from, to time.Time) []Point {
	if len(ms) == 0 || to.Before(from) {
		return nil
	}
	byDay := make(map[string]float64, len(ms))
	sorted := append([]Measurement(nil), ms...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })
	for _, m := range sorted {
		byDay[day(m.Date)] = m.Kg // повтор за ту же дату: побеждает последний
	}

	start := truncate(sorted[0].Date)
	if from.Before(start) {
		from = start
	}
	from, to = truncate(from), truncate(to)

	var out []Point
	trend := 0.0
	started := false
	for d := start; !d.After(to); d = d.AddDate(0, 0, 1) {
		w, measured := byDay[day(d)]
		switch {
		case measured && !started:
			trend, started = w, true // первое значение тренда равно первому замеру
		case measured:
			trend += Alpha * (w - trend)
		}
		if d.Before(from) {
			continue
		}
		p := Point{Date: d}
		if started {
			p.Trend = round1(trend)
		}
		if measured {
			p.Weight = w
		}
		out = append(out, p)
	}
	return out
}

// RatePerWeek — скорость изменения веса в кг в неделю по линии тренда за окно.
// Считается по краям окна, а не по замерам: тренд уже сглажен, и этого достаточно.
// Меньше двух дней с трендом — скорости нет (0, false).
func RatePerWeek(points []Point) (float64, bool) {
	var first, last *Point
	for i := range points {
		if points[i].Trend == 0 {
			continue
		}
		if first == nil {
			first = &points[i]
		}
		last = &points[i]
	}
	if first == nil || last == nil || first == last {
		return 0, false
	}
	days := last.Date.Sub(first.Date).Hours() / 24
	if days < 1 {
		return 0, false
	}
	return round2((last.Trend - first.Trend) / days * 7), true
}

// Coverage — сколько дней окна покрыто данными. Нужен фазе 2: пересчитывать норму
// по неполным данным нельзя, ошибка на 300 ккал превращается в полкило в месяц мимо цели.
type Coverage struct {
	Days     int `json:"days"`     // длина окна
	WeighIns int `json:"weighIns"` // дней с замером веса
	FreshDay int `json:"freshDay"` // сколько дней назад последний замер; -1 — замеров нет
}

// Cover считает покрытие замерами веса за окно [from, to] включительно.
func Cover(ms []Measurement, from, to time.Time) Coverage {
	from, to = truncate(from), truncate(to)
	c := Coverage{Days: int(to.Sub(from).Hours()/24) + 1, FreshDay: -1}
	if c.Days < 0 {
		c.Days = 0
	}
	seen := map[string]bool{}
	var latest time.Time
	for _, m := range ms {
		d := truncate(m.Date)
		if d.Before(from) || d.After(to) || seen[day(d)] {
			continue
		}
		seen[day(d)] = true
		c.WeighIns++
		if latest.IsZero() || d.After(latest) {
			latest = d
		}
	}
	if !latest.IsZero() {
		c.FreshDay = int(to.Sub(latest).Hours() / 24)
	}
	return c
}

// ReadyToAdapt отвечает на единственный вопрос фазы 2: можно ли пересчитывать норму.
// Пороги по окну в 14 дней: не меньше 4 замеров и хотя бы один за последние 3 дня.
// Если нет — возвращает коды причин, чтобы интерфейс показал, чего не хватает, а не молчал.
func ReadyToAdapt(c Coverage) (bool, []string) {
	var why []string
	if c.WeighIns < 4 {
		why = append(why, "few_weighins")
	}
	if c.FreshDay < 0 || c.FreshDay > 3 {
		why = append(why, "stale_weighin")
	}
	return len(why) == 0, why
}

func truncate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func day(t time.Time) string { return truncate(t).Format("2006-01-02") }

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func round2(v float64) float64 { return math.Round(v*100) / 100 }
