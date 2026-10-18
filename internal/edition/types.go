package edition

import "edition/internal/domain"

const (
	RoleReporter = domain.RoleReporter
	RoleEditor   = domain.RoleEditor
)

type Config = domain.Config
type Receipt = domain.Receipt
type Notification = domain.Notification
type ClientStats = domain.ClientStats

func DefaultConfig() Config {
	return domain.DefaultConfig()
}

type Request struct {
	Type       string `json:"type"`
	ClientID   string `json:"client_id,omitempty"`
	Secret     string `json:"secret,omitempty"`
	NoteID     string `json:"note_id,omitempty"`
	Section    string `json:"section,omitempty"`
	Topic      string `json:"topic,omitempty"`
	OccurredAt string `json:"occurred_at,omitempty"`
	All        bool   `json:"all,omitempty"`
	MessageID  int64  `json:"message_id,omitempty"`
}

type Response struct {
	Type     string         `json:"type"`
	OK       bool           `json:"ok"`
	Error    string         `json:"error,omitempty"`
	Role     string         `json:"role,omitempty"`
	Receipt  *Receipt       `json:"receipt,omitempty"`
	Messages []Notification `json:"messages,omitempty"`
	Stats    []ClientStats  `json:"stats,omitempty"`
}
