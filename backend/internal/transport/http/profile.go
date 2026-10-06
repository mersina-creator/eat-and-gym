package http

import (
	"net/http"

	"racion/internal/domain"
)

// ФТ-01: профиль для расчёта нормы. GET отдаёт {profile, targets} или null, PUT сохраняет и
// возвращает пересчитанную норму — фронту не нужен второй запрос.

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	u := requireUser(w, r)
	if u == nil {
		return
	}
	v, err := s.svc.Profiles.Get(r.Context(), u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) putProfile(w http.ResponseWriter, r *http.Request) {
	u := requireUser(w, r)
	if u == nil {
		return
	}
	var p domain.UserProfile
	if !decode(w, r, 4<<10, &p) {
		return
	}
	v, err := s.svc.Profiles.Save(r.Context(), u.ID, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, v)
}
