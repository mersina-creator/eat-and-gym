package domain

import "time"

// WeightEntry — одно взвешивание. Date — календарная дата без времени: вес за 6 октября
// остаётся весом за 6 октября независимо от часового пояса сервера.
type WeightEntry struct {
	Date time.Time `json:"date"`
	Kg   float64   `json:"kg"`
	Note string    `json:"note,omitempty"`
}
