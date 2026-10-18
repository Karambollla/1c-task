package domain

import (
	"errors"
	"math"
	"time"
)

type Contribution struct {
	Trust      float64
	OccurredAt time.Time
}

func ValidateOccurredAt(raw string, now time.Time) (time.Time, error) {
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, errors.New("occurred_at must be RFC3339 UTC")
	}
	_, offset := at.Zone()
	if offset != 0 || at.After(now.UTC()) {
		return time.Time{}, errors.New("occurred_at cannot be in the future and must be UTC")
	}
	return at.UTC(), nil
}

func Score(contributions []Contribution, now time.Time, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		return 0
	}
	var total float64
	for _, c := range contributions {
		age := now.Sub(c.OccurredAt)
		total += c.Trust * math.Pow(0.5, age.Hours()/halfLife.Hours())
	}
	return total
}
