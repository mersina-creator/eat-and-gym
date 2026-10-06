package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"racion/internal/domain"
	"racion/internal/journal"
)

// ФТ-05, шаг 1: журнал веса. Вес — самый честный сигнал прогресса: его не надо вспоминать
// и не надо оценивать на глаз, в отличие от съеденного. Поэтому он выкатывается первым
// и начинает копить историю до того, как появится остальной журнал.
//
// Арифметика живёт в internal/journal и покрыта тестами, здесь только сценарии:
// проверить вход, сходить в базу, собрать ответ.

// JournalRepo — порт хранилища журнала.
type JournalRepo interface {
	SetWeight(ctx context.Context, userID string, date time.Time, kg float64, note string) error
	DeleteWeight(ctx context.Context, userID string, date time.Time) (bool, error)
	Weights(ctx context.Context, userID string, from time.Time) ([]domain.WeightEntry, error)
	LastWeight(ctx context.Context, userID string) (domain.WeightEntry, bool, error)
}

type Journal struct{ repo JournalRepo }

func NewJournal(repo JournalRepo) *Journal { return &Journal{repo: repo} }

// Границы разумного веса. Совпадают с проверкой в базе: ошибка должна ловиться до SQL,
// чтобы человек увидел понятный текст, а не отказ сервера.
const (
	minKg = 30.0
	maxKg = 400.0
	// Глубже двух лет замеры не принимаем: это почти всегда опечатка в годе.
	maxBackYears = 2
)

// WeightView — ответ экрана журнала: ряд дней с трендом плюс сводка.
type WeightView struct {
	From     string           `json:"from"`
	To       string           `json:"to"`
	Points   []journal.Point  `json:"points"`
	Last     *domain.WeightEntry `json:"last,omitempty"`     // последний замер, для подсказки в поле ввода
	Rate     *float64         `json:"ratePerWeek,omitempty"` // кг в неделю по тренду; нет — данных мало
	Coverage journal.Coverage `json:"coverage"`
	Ready    bool             `json:"ready"`             // можно ли пересчитывать норму (фаза 2)
	Blockers []string         `json:"blockers,omitempty"` // чего не хватает: few_weighins, stale_weighin
}

// View собирает окно в days дней, заканчивающееся сегодняшней датой клиента.
// today приходит с устройства: сервер не вычисляет дату из своего времени.
func (j *Journal) View(ctx context.Context, userID string, today time.Time, days int) (WeightView, error) {
	if days <= 0 || days > 365 {
		days = 14
	}
	today = truncate(today)
	from := today.AddDate(0, 0, -(days - 1))

	// Берём всю историю, а не только окно: тренд в начале окна должен продолжать прошлое,
	// иначе первая точка каждый раз стартует заново и график врёт.
	all, err := j.repo.Weights(ctx, userID, time.Time{})
	if err != nil {
		return WeightView{}, err
	}

	ms := make([]journal.Measurement, 0, len(all))
	for _, e := range all {
		ms = append(ms, journal.Measurement{Date: e.Date, Kg: e.Kg})
	}

	v := WeightView{
		From:     from.Format(dateLayout),
		To:       today.Format(dateLayout),
		Points:   journal.Trend(ms, from, today),
		Coverage: journal.Cover(ms, from, today),
	}
	if rate, ok := journal.RatePerWeek(v.Points); ok {
		v.Rate = &rate
	}
	v.Ready, v.Blockers = journal.ReadyToAdapt(v.Coverage)

	if last, ok, err := j.repo.LastWeight(ctx, userID); err != nil {
		return WeightView{}, err
	} else if ok {
		v.Last = &last
	}
	return v, nil
}

// SetWeight записывает замер за дату. today — сегодняшняя дата клиента, нужна для проверки
// «не в будущем»: без неё сервер в другом часовом поясе отверг бы сегодняшний вес.
func (j *Journal) SetWeight(ctx context.Context, userID string, date, today time.Time, kg float64, note string) error {
	if err := checkWeight(date, today, kg); err != nil {
		return err
	}
	return j.repo.SetWeight(ctx, userID, truncate(date), round1(kg), strings.TrimSpace(note))
}

// DeleteWeight убирает замер. Отсутствие замера — не ошибка сервера, а ErrNotFound.
func (j *Journal) DeleteWeight(ctx context.Context, userID string, date time.Time) error {
	ok, err := j.repo.DeleteWeight(ctx, userID, truncate(date))
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	return nil
}

// ImportResult — итог импорта истории: что легло и что не разобралось.
type ImportResult struct {
	Added  int      `json:"added"`
	Errors []string `json:"errors,omitempty"`
}

// Import принимает историю списком: строка «дата вес». Разделитель — пробел, запятая
// или табуляция, дробная часть через точку или запятую.
//
// Отката нет: что разобралось, то записано. Человек, переносящий полгода записей из заметок,
// не должен терять всё из-за одной кривой строки — он должен увидеть, какие строки поправить.
func (j *Journal) Import(ctx context.Context, userID string, today time.Time, text string) (ImportResult, error) {
	var res ImportResult
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		date, kg, err := parseWeightLine(line)
		if err == nil {
			err = checkWeight(date, today, kg)
		}
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("строка %d: %s", i+1, err))
			continue
		}
		if err := j.repo.SetWeight(ctx, userID, date, round1(kg), ""); err != nil {
			return res, err
		}
		res.Added++
	}
	return res, nil
}

const dateLayout = "2006-01-02"

// parseWeightLine разбирает строку вида «2026-09-01 134,2» или «2026-09-01, 134.2».
func parseWeightLine(line string) (time.Time, float64, error) {
	f := strings.FieldsFunc(line, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == ';'
	})
	// запятая как десятичный разделитель даёт три поля: дата, целая часть, дробная
	if len(f) == 3 && len(f[2]) <= 2 {
		f = []string{f[0], f[1] + "." + f[2]}
	}
	if len(f) < 2 {
		return time.Time{}, 0, fmt.Errorf("ждём «дата вес»")
	}
	date, err := time.Parse(dateLayout, f[0])
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("дата «%s» не в формате ГГГГ-ММ-ДД", f[0])
	}
	kg, err := strconv.ParseFloat(strings.Replace(f[1], ",", ".", 1), 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("вес «%s» не число", f[1])
	}
	return date, kg, nil
}

func checkWeight(date, today time.Time, kg float64) error {
	if kg < minKg || kg > maxKg {
		return fmt.Errorf("%w: вес вне диапазона %.0f–%.0f кг", domain.ErrBadInput, minKg, maxKg)
	}
	date, today = truncate(date), truncate(today)
	if date.After(today) {
		return fmt.Errorf("%w: дата в будущем", domain.ErrBadInput)
	}
	if date.Before(today.AddDate(-maxBackYears, 0, 0)) {
		return fmt.Errorf("%w: дата старше %d лет", domain.ErrBadInput, maxBackYears)
	}
	return nil
}

func truncate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
