package domain

// UserProfile — данные для расчёта нормы калорий и БЖУ (ФТ-01). Даты — YYYY-MM-DD.
type UserProfile struct {
	Sex        string  `json:"sex"` // m | f
	BirthDate  string  `json:"birthDate"`
	HeightCm   float64 `json:"heightCm"`
	WeightKg   float64 `json:"weightKg"`
	BodyFatPct float64 `json:"bodyFatPct,omitempty"` // 0 — неизвестен
	Activity   string  `json:"activity"`             // sedentary | light | moderate | high | extreme
	Goal       string  `json:"goal"`                 // lose | recomp | maintain | gain
	Pace       string  `json:"pace"`                 // slow | normal | fast
	UpdatedAt  string  `json:"updatedAt,omitempty"`
}
