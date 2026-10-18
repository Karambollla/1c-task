package edition

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"edition/internal/domain"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type Store struct {
	db  *sql.DB
	cfg Config
	now func() time.Time
}

type publication struct {
	storyID    int64
	section    string
	topic      string
	releasedAt time.Time
}

func Open(path string, cfg Config) (*Store, error) {
	db, err := sql.Open(
		"sqlite",
		path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
	)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, cfg: cfg, now: time.Now}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY, role TEXT NOT NULL CHECK(role IN ('reporter','editor')),
  secret_hash BLOB NOT NULL, trust REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS stories (
  id INTEGER PRIMARY KEY AUTOINCREMENT, section TEXT NOT NULL COLLATE BINARY,
  topic TEXT NOT NULL COLLATE BINARY, created_at TEXT NOT NULL,
  UNIQUE(section, topic)
);
CREATE TABLE IF NOT EXISTS notes (
  id INTEGER PRIMARY KEY AUTOINCREMENT, client_id TEXT NOT NULL REFERENCES users(id),
  note_id TEXT NOT NULL, fingerprint TEXT NOT NULL, status TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '', story_id INTEGER REFERENCES stories(id),
  event_at TEXT, received_at TEXT NOT NULL, UNIQUE(client_id, note_id)
);
CREATE TABLE IF NOT EXISTS voices (
  story_id INTEGER NOT NULL REFERENCES stories(id), client_id TEXT NOT NULL REFERENCES users(id),
  first_note_pk INTEGER NOT NULL REFERENCES notes(id), trust_snapshot REAL NOT NULL,
  event_at TEXT NOT NULL, PRIMARY KEY(story_id, client_id)
);
CREATE TABLE IF NOT EXISTS publications (
  story_id INTEGER PRIMARY KEY REFERENCES stories(id), trigger_note_pk INTEGER NOT NULL REFERENCES notes(id),
  released_at TEXT NOT NULL, score REAL NOT NULL
);
CREATE TABLE IF NOT EXISTS outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT, client_id TEXT NOT NULL REFERENCES users(id),
  kind TEXT NOT NULL, text TEXT NOT NULL, created_at TEXT NOT NULL,
  acknowledged_at TEXT
);
CREATE INDEX IF NOT EXISTS notes_client_idx ON notes(client_id, status);
CREATE INDEX IF NOT EXISTS notes_story_idx ON notes(story_id, id);
CREATE INDEX IF NOT EXISTS outbox_pending_idx ON outbox(client_id, acknowledged_at, id);`)
	return err
}

func (s *Store) CreateUser(ctx context.Context, id, role, secret string, trust float64) error {
	if id == "" || secret == "" {
		return errors.New("id and secret are required")
	}
	if role != RoleReporter && role != RoleEditor {
		return errors.New("unknown role")
	}
	if trust < 0 {
		return errors.New("trust must not be negative")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO users(id,role,secret_hash,trust,created_at) VALUES(?,?,?,?,?)`,
		id,
		role,
		hash,
		trust,
		stamp(s.now()),
	)
	return err
}

func (s *Store) SetTrust(ctx context.Context, id string, trust float64) error {
	if trust < 0 {
		return errors.New("trust must not be negative")
	}
	r, err := s.db.ExecContext(ctx, `UPDATE users SET trust=? WHERE id=? AND role=?`, trust, id, RoleReporter)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("reporter %q not found", id)
	}
	return nil
}

func (s *Store) Authenticate(ctx context.Context, id, secret string) (string, error) {
	var role string
	var hash []byte
	err := s.db.QueryRowContext(ctx, `SELECT role,secret_hash FROM users WHERE id=?`, id).Scan(&role, &hash)
	if err != nil {
		return "", errors.New("unknown client or invalid secret")
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(secret)) != nil {
		return "", errors.New("unknown client or invalid secret")
	}
	return role, nil
}

func fingerprint(section, topic, occurred string) string {
	b, _ := json.Marshal(struct{ Section, Topic, Occurred string }{section, topic, occurred})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func stamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseStamp(v string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, v)
}

func (s *Store) Submit(ctx context.Context, submission domain.Submission) (Receipt, error) {
	if submission.NoteID == "" {
		return Receipt{Status: "rejected", Reason: "note_id is required"}, nil
	}
	if submission.Section == "" || submission.Topic == "" {
		return s.recordRejected(
			ctx,
			submission,
			fingerprint(submission.Section, submission.Topic, submission.OccurredAt),
			"section and topic are required",
		)
	}
	now := s.now().UTC()
	eventAt, err := domain.ValidateOccurredAt(submission.OccurredAt, now)
	if err != nil {
		return s.recordRejected(
			ctx,
			submission,
			fingerprint(submission.Section, submission.Topic, submission.OccurredAt),
			err.Error(),
		)
	}
	fp := fingerprint(submission.Section, submission.Topic, submission.OccurredAt)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.Rollback()
	var oldFP, oldStatus, oldReason string
	var oldStory sql.NullInt64
	err = tx.QueryRowContext(
		ctx,
		`SELECT fingerprint,status,reason,story_id FROM notes WHERE client_id=? AND note_id=?`,
		submission.ClientID,
		submission.NoteID,
	).Scan(&oldFP, &oldStatus, &oldReason, &oldStory)
	if err == nil {
		if oldFP != fp {
			return Receipt{
				Status: "rejected",
				NoteID: submission.NoteID,
				Reason: "note_id was already used with different data",
			}, nil
		}
		return Receipt{
			Status:  "duplicate",
			NoteID:  submission.NoteID,
			Reason:  oldReason,
			StoryID: oldStory.Int64,
		}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Receipt{}, err
	}
	var trust float64
	if err := tx.QueryRowContext(
		ctx,
		`SELECT trust FROM users WHERE id=? AND role=?`,
		submission.ClientID,
		RoleReporter,
	).Scan(&trust); err != nil {
		return Receipt{}, errors.New("only reporters may submit notes")
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO stories(section,topic,created_at) VALUES(?,?,?) ON CONFLICT(section,topic) DO NOTHING`,
		submission.Section,
		submission.Topic,
		stamp(now),
	)
	if err != nil {
		return Receipt{}, err
	}
	var storyID int64
	if err = tx.QueryRowContext(
		ctx,
		`SELECT id FROM stories WHERE section=? AND topic=?`,
		submission.Section,
		submission.Topic,
	).Scan(&storyID); err != nil {
		return Receipt{}, err
	}
	r, err := tx.ExecContext(
		ctx,
		`INSERT INTO notes(client_id,note_id,fingerprint,status,story_id,event_at,received_at) VALUES(?,?,?,?,?,?,?)`,
		submission.ClientID,
		submission.NoteID,
		fp,
		"accepted",
		storyID,
		stamp(eventAt),
		stamp(now),
	)
	if err != nil {
		return Receipt{}, err
	}
	notePK, _ := r.LastInsertId()
	var published int
	if err = tx.QueryRowContext(
		ctx,
		`SELECT EXISTS(SELECT 1 FROM publications WHERE story_id=?)`,
		storyID,
	).Scan(&published); err != nil {
		return Receipt{}, err
	}
	score := 0.0
	if published == 0 {
		var exists int
		if err = tx.QueryRowContext(
			ctx,
			`SELECT EXISTS(SELECT 1 FROM voices WHERE story_id=? AND client_id=?)`,
			storyID,
			submission.ClientID,
		).Scan(&exists); err != nil {
			return Receipt{}, err
		}
		if exists == 0 {
			if _, err = tx.ExecContext(
				ctx,
				`INSERT INTO voices(story_id,client_id,first_note_pk,trust_snapshot,event_at) VALUES(?,?,?,?,?)`,
				storyID,
				submission.ClientID,
				notePK,
				trust,
				stamp(eventAt),
			); err != nil {
				return Receipt{}, err
			}
		}
		score, err = s.scoreTx(ctx, tx, storyID, now)
		if err != nil {
			return Receipt{}, err
		}
		if score >= s.cfg.Threshold {
			if _, err = tx.ExecContext(
				ctx,
				`INSERT INTO publications(story_id,trigger_note_pk,released_at,score) VALUES(?,?,?,?)`,
				storyID,
				notePK,
				stamp(now),
				score,
			); err != nil {
				return Receipt{}, err
			}
			if err = s.enqueueCongratulations(ctx, tx, publication{
				storyID:    storyID,
				section:    submission.Section,
				topic:      submission.Topic,
				releasedAt: now,
			}); err != nil {
				return Receipt{}, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return Receipt{}, err
	}
	return Receipt{
		Status:  "accepted",
		NoteID:  submission.NoteID,
		StoryID: storyID,
		Score:   score,
	}, nil
}

func (s *Store) recordRejected(ctx context.Context, submission domain.Submission, fp, reason string) (Receipt, error) {
	if submission.NoteID == "" {
		return Receipt{Status: "rejected", Reason: reason}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.Rollback()
	var oldFP string
	err = tx.QueryRowContext(
		ctx,
		`SELECT fingerprint FROM notes WHERE client_id=? AND note_id=?`,
		submission.ClientID,
		submission.NoteID,
	).Scan(&oldFP)
	if err == nil {
		if oldFP == fp {
			return Receipt{Status: "duplicate", NoteID: submission.NoteID}, nil
		}
		return Receipt{
			Status: "rejected",
			NoteID: submission.NoteID,
			Reason: "note_id was already used with different data",
		}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Receipt{}, err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO notes(client_id,note_id,fingerprint,status,reason,received_at) VALUES(?,?,?,?,?,?)`,
		submission.ClientID,
		submission.NoteID,
		fp,
		"rejected",
		reason,
		stamp(s.now()),
	)
	if err != nil {
		return Receipt{}, err
	}
	if err = tx.Commit(); err != nil {
		return Receipt{}, err
	}
	return Receipt{Status: "rejected", NoteID: submission.NoteID, Reason: reason}, nil
}

func (s *Store) scoreTx(ctx context.Context, tx *sql.Tx, storyID int64, now time.Time) (float64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT trust_snapshot,event_at FROM voices WHERE story_id=?`, storyID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var contributions []domain.Contribution
	for rows.Next() {
		var trust float64
		var raw string
		if err := rows.Scan(&trust, &raw); err != nil {
			return 0, err
		}
		at, err := parseStamp(raw)
		if err != nil {
			return 0, err
		}
		contributions = append(contributions, domain.Contribution{Trust: trust, OccurredAt: at})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return domain.Score(contributions, now, s.cfg.HalfLife), nil
}

func (s *Store) enqueueCongratulations(ctx context.Context, tx *sql.Tx, publication publication) error {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT client_id FROM voices WHERE story_id=? ORDER BY first_note_pk LIMIT ?`,
		publication.storyID,
		s.cfg.Congratulations,
	)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for rank, id := range ids {
		text := fmt.Sprintf(
			"Поздравляем, ваша заметка попала в печать. Вы были %d-м, "+
				"кто сказал нам об этой новости: [%s] %s.",
			rank+1,
			publication.section,
			publication.topic,
		)
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO outbox(client_id,kind,text,created_at) VALUES(?,?,?,?)`,
			id,
			"congratulation",
			text,
			stamp(publication.releasedAt),
		); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) Pending(ctx context.Context, clientID string) ([]Notification, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id,kind,text,created_at FROM outbox WHERE client_id=? AND acknowledged_at IS NULL ORDER BY id`,
		clientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		var raw string
		if err := rows.Scan(&n.ID, &n.Kind, &n.Text, &raw); err != nil {
			return nil, err
		}
		n.Created, _ = parseStamp(raw)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) Ack(ctx context.Context, clientID string, id int64) error {
	r, err := s.db.ExecContext(
		ctx,
		`UPDATE outbox SET acknowledged_at=? WHERE id=? AND client_id=? AND acknowledged_at IS NULL`,
		stamp(s.now()),
		id,
		clientID,
	)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return errors.New("pending message not found")
	}
	return nil
}

func (s *Store) Stats(ctx context.Context, caller string, all bool) ([]ClientStats, error) {
	if all {
		var role string
		if err := s.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id=?`, caller).Scan(&role); err != nil {
			return nil, err
		}
		if role != RoleEditor {
			return nil, errors.New("only editor may request all statistics")
		}
	}
	q := `SELECT id FROM users`
	args := []any{}
	if !all {
		q += ` WHERE id=?`
		args = append(args, caller)
	}
	q += ` ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var out []ClientStats
	for _, id := range ids {
		st, err := s.statsFor(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *Store) statsFor(ctx context.Context, id string) (ClientStats, error) {
	st := ClientStats{ClientID: id}
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM notes WHERE client_id=? AND status='accepted'`,
		id,
	).Scan(&st.Accepted); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM notes n JOIN publications p ON p.story_id=n.story_id
		 WHERE n.client_id=? AND n.status='accepted' AND n.id<=p.trigger_note_pk`,
		id,
	).Scan(&st.Printed); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(DISTINCT story_id) FROM notes WHERE client_id=? AND status='accepted'`,
		id,
	).Scan(&st.Stories); err != nil {
		return st, err
	}
	if st.Accepted > 0 {
		st.Share = float64(st.Printed) / float64(st.Accepted)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT (SELECT COUNT(*) FROM voices earlier
        WHERE earlier.story_id=v.story_id AND earlier.first_note_pk<=v.first_note_pk)
FROM voices v JOIN publications p ON p.story_id=v.story_id WHERE v.client_id=?`, id)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	var total, contributions int
	for rows.Next() {
		var rank int
		if err := rows.Scan(&rank); err != nil {
			return st, err
		}
		contributions++
		total += rank
		if rank == 1 {
			st.Sensations++
		}
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	if contributions > 0 {
		st.AverageRank = float64(total) / float64(contributions)
	}
	return st, nil
}
