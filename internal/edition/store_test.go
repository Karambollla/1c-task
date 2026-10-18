package edition

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T, threshold float64) (*Store, time.Time) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "edition.db"), Config{Threshold: threshold, HalfLife: 6 * time.Hour, Congratulations: 3})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, now
}

func addReporter(t *testing.T, s *Store, id string, trust float64) {
	t.Helper()
	if err := s.CreateUser(context.Background(), id, RoleReporter, id+"-secret", trust); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationIdempotencyStatsAndOutbox(t *testing.T) {
	s, now := testStore(t, 1.05)
	addReporter(t, s, "first", .70)
	addReporter(t, s, "second", .50)
	stamp := now.Format(time.RFC3339)

	first, err := s.Submit(context.Background(), "first", "11111111-1111-4111-8111-111111111111", "City", "Bridge is closed", stamp)
	if err != nil || first.Status != "accepted" || first.Score != .70 {
		t.Fatalf("first receipt = %#v, %v", first, err)
	}
	duplicate, err := s.Submit(context.Background(), "first", "11111111-1111-4111-8111-111111111111", "City", "Bridge is closed", stamp)
	if err != nil || duplicate.Status != "duplicate" {
		t.Fatalf("duplicate receipt = %#v, %v", duplicate, err)
	}
	conflict, err := s.Submit(context.Background(), "first", "11111111-1111-4111-8111-111111111111", "City", "Different topic", stamp)
	if err != nil || conflict.Status != "rejected" {
		t.Fatalf("conflict receipt = %#v, %v", conflict, err)
	}
	second, err := s.Submit(context.Background(), "second", "22222222-2222-4222-8222-222222222222", "City", "Bridge is closed", stamp)
	if err != nil || second.Status != "accepted" || second.Score < 1.19 {
		t.Fatalf("second receipt = %#v, %v", second, err)
	}

	items, err := s.Pending(context.Background(), "first")
	if err != nil || len(items) != 1 {
		t.Fatalf("first outbox = %#v, %v", items, err)
	}
	if err = s.Ack(context.Background(), "first", items[0].ID); err != nil {
		t.Fatal(err)
	}
	items, err = s.Pending(context.Background(), "first")
	if err != nil || len(items) != 0 {
		t.Fatalf("ack did not clear outbox: %#v, %v", items, err)
	}

	_, err = s.Submit(context.Background(), "first", "33333333-3333-4333-8333-333333333333", "City", "Bridge is closed", stamp)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats(context.Background(), "first", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := stats[0]; got.Accepted != 2 || got.Printed != 1 || got.Stories != 1 || got.Sensations != 1 || got.AverageRank != 1 {
		t.Fatalf("unexpected first statistics: %#v", got)
	}
}

func TestOldFutureAndStrictStories(t *testing.T) {
	s, now := testStore(t, .5)
	addReporter(t, s, "a", .5)
	addReporter(t, s, "b", .5)
	future, err := s.Submit(context.Background(), "a", "future", "News", "Topic", now.Add(time.Second).Format(time.RFC3339))
	if err != nil || future.Status != "rejected" {
		t.Fatalf("future = %#v, %v", future, err)
	}
	old, err := s.Submit(context.Background(), "a", "old", "News", "Topic", now.Add(-365*24*time.Hour).Format(time.RFC3339))
	if err != nil || old.Status != "accepted" || old.Score >= .01 {
		t.Fatalf("old = %#v, %v", old, err)
	}
	other, err := s.Submit(context.Background(), "b", "case", "News", "topic", now.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if old.StoryID == other.StoryID {
		t.Fatalf("case-distinct topics were merged into story %d", old.StoryID)
	}
}

func TestTrustIsSnapshottedAndOnlyFirstVoiceCounts(t *testing.T) {
	s, now := testStore(t, 1.05)
	addReporter(t, s, "a", .40)
	addReporter(t, s, "b", .50)
	addReporter(t, s, "c", .20)
	stamp := now.Format(time.RFC3339)
	if _, err := s.Submit(context.Background(), "a", "a1", "News", "Shared", stamp); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTrust(context.Background(), "a", .90); err != nil {
		t.Fatal(err)
	}
	second, err := s.Submit(context.Background(), "b", "b1", "News", "Shared", stamp)
	if err != nil || second.Score > .91 {
		t.Fatalf("trust change rewrote a historical vote: %#v, %v", second, err)
	}
	third, err := s.Submit(context.Background(), "c", "c1", "News", "Shared", stamp)
	if err != nil || third.Score < 1.09 {
		t.Fatalf("third = %#v, %v", third, err)
	}
}
