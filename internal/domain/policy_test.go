package domain

import (
	"math"
	"testing"
	"time"
)

func TestScoreUsesEventTimeAndHalfLife(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	score := Score([]Contribution{{Trust: 1, OccurredAt: now.Add(-6 * time.Hour)}}, now, 6*time.Hour)
	if math.Abs(score-.5) > 1e-12 {
		t.Fatalf("score = %v, want 0.5", score)
	}
}

func TestValidateOccurredAt(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	if _, err := ValidateOccurredAt("2026-10-18T12:00:01Z", now); err == nil {
		t.Fatal("future time was accepted")
	}
	if _, err := ValidateOccurredAt("2026-10-18T12:00:00+03:00", now); err == nil {
		t.Fatal("non-UTC offset was accepted")
	}
	at, err := ValidateOccurredAt("2026-10-18T12:00:00+00:00", now)
	if err != nil || at.Location() != time.UTC {
		t.Fatalf("UTC timestamp = %v, %v", at, err)
	}
}
