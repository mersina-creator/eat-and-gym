package http

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"racion/internal/domain"
)

// ФТ-05, шаг 1: журнал веса.
//
// Дату клиент присылает сам — и сегодняшнюю (поле today), и дату замера. Сервер не берёт
// её из своего времени: взвешивание в 23:50 по Москве иначе уехало бы на следующие сутки по UTC.

const dateLayout = "2006-01-02"

// parseDay читает дату из пути или тела. Пустая строка — не ошибка, вернёт нулевое время.
func parseDay(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// clientToday — сегодняшняя дата клиента из query-параметра today; без него берём дату сервера.
func clientToday(r *http.Request) time.Time {
	if t, ok := parseDay(r.URL.Query().Get("today")); ok {
		return t
	}
	return time.Now().UTC()
}

// badInput отдаёт 422 с внятным текстом вместо общего «bad json»: человек, который
// ввёл вес 1330 вместо 133,0, должен увидеть, что именно не так.
func (s *Server) badInput(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, domain.ErrBadInput) {
		writeErr(w, 422, err.Error())
		return true
	}
	return false
}

// GET /api/me/journal/weight?days=14&today=2026-10-06
func (s *Server) weightJournal(w http.ResponseWriter, r *http.Request) {
	u := requireUser(w, r)
	if u == nil {
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	view, err := s.svc.Journal.View(r.Context(), u.ID, clientToday(r), days)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, view)
}

// PUT /api/me/weight/{date}
func (s *Server) setWeight(w http.ResponseWriter, r *http.Request) {
	u := requireUser(w, r)
	if u == nil {
		return
	}
	date, ok := parseDay(r.PathValue("date"))
	if !ok {
		writeErr(w, 422, "дата не в формате ГГГГ-ММ-ДД")
		return
	}
	var body struct {
		Kg    float64 `json:"kg"`
		Note  string  `json:"note"`
		Today string  `json:"today"`
	}
	if !decode(w, r, 4<<10, &body) {
		return
	}
	today := clientToday(r)
	if t, ok := parseDay(body.Today); ok {
		today = t
	}
	if err := s.svc.Journal.SetWeight(r.Context(), u.ID, date, today, body.Kg, body.Note); err != nil {
		if s.badInput(w, r, err) {
			return
		}
		s.fail(w, r, err)
		return
	}
	view, err := s.svc.Journal.View(r.Context(), u.ID, today, 14)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, view)
}

// DELETE /api/me/weight/{date}
func (s *Server) deleteWeight(w http.ResponseWriter, r *http.Request) {
	u := requireUser(w, r)
	if u == nil {
		return
	}
	date, ok := parseDay(r.PathValue("date"))
	if !ok {
		writeErr(w, 422, "дата не в формате ГГГГ-ММ-ДД")
		return
	}
	if err := s.svc.Journal.DeleteWeight(r.Context(), u.ID, date); err != nil {
		s.fail(w, r, err)
		return
	}
	view, err := s.svc.Journal.View(r.Context(), u.ID, clientToday(r), 14)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, view)
}

// POST /api/me/weight/import — история списком «дата вес» построчно.
func (s *Server) importWeight(w http.ResponseWriter, r *http.Request) {
	u := requireUser(w, r)
	if u == nil {
		return
	}
	var body struct {
		Text  string `json:"text"`
		Today string `json:"today"`
	}
	if !decode(w, r, 256<<10, &body) {
		return
	}
	today := clientToday(r)
	if t, ok := parseDay(body.Today); ok {
		today = t
	}
	res, err := s.svc.Journal.Import(r.Context(), u.ID, today, body.Text)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view, err := s.svc.Journal.View(r.Context(), u.ID, today, 14)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"result": res, "view": view})
}
