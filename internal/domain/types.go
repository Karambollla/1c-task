package domain

import "time"

const (
	RoleReporter = "reporter"
	RoleEditor   = "editor"
)

type Config struct {
	Threshold       float64
	HalfLife        time.Duration
	Congratulations int
}

func DefaultConfig() Config {
	return Config{Threshold: 1.05, HalfLife: 6 * time.Hour, Congratulations: 3}
}

type Receipt struct {
	Status  string  `json:"status"`
	NoteID  string  `json:"note_id"`
	Reason  string  `json:"reason,omitempty"`
	StoryID int64   `json:"story_id,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

type Submission struct {
	ClientID   string
	NoteID     string
	Section    string
	Topic      string
	OccurredAt string
}

type Notification struct {
	ID      int64     `json:"id"`
	Kind    string    `json:"kind"`
	Text    string    `json:"text"`
	Created time.Time `json:"created_at"`
}

type ClientStats struct {
	ClientID    string  `json:"client_id"`
	Accepted    int     `json:"accepted"`
	Printed     int     `json:"printed"`
	Share       float64 `json:"share"`
	Stories     int     `json:"stories"`
	Sensations  int     `json:"sensations"`
	AverageRank float64 `json:"average_rank"`
}
