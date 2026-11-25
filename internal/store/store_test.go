package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertGetUpdateDelete(t *testing.T) {
	s := openTest(t)
	now := time.Now().Truncate(time.Microsecond)
	j := Job{ID: "a1", Name: "hello", Cmd: "echo hi", Status: StatusPending, LogPath: "/tmp/a1.log", CreatedAt: now}
	if err := s.Insert(j); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("a1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "hello" || got.Status != StatusPending || !got.CreatedAt.Equal(now) || !got.StartedAt.IsZero() {
		t.Fatalf("unexpected job: %+v", got)
	}

	got.Status, got.ExitCode, got.StartedAt, got.FinishedAt = StatusFailed, 3, now, now.Add(time.Second)
	if err := s.Update(got); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.Get("a1")
	if got2.Status != StatusFailed || got2.ExitCode != 3 || !got2.FinishedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("update not saved: %+v", got2)
	}

	if err := s.Delete("a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("a1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Delete("a1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: want ErrNotFound, got %v", err)
	}
}

func TestListNewestFirst(t *testing.T) {
	s := openTest(t)
	base := time.Now()
	for i, id := range []string{"old", "mid", "new"} {
		if err := s.Insert(Job{ID: id, Name: id, Cmd: "true", Status: StatusCompleted, CreatedAt: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 || jobs[0].ID != "new" || jobs[2].ID != "old" {
		t.Fatalf("wrong order: %v", jobs)
	}
}

func TestMarkInterrupted(t *testing.T) {
	s := openTest(t)
	now := time.Now()
	for id, st := range map[string]Status{"p": StatusPending, "r": StatusRunning, "c": StatusCompleted} {
		if err := s.Insert(Job{ID: id, Name: id, Cmd: "true", Status: st, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.MarkInterrupted(now)
	if err != nil || n != 2 {
		t.Fatalf("MarkInterrupted = %d, %v; want 2", n, err)
	}
	for id, want := range map[string]Status{"p": StatusInterrupted, "r": StatusInterrupted, "c": StatusCompleted} {
		j, _ := s.Get(id)
		if j.Status != want {
			t.Errorf("%s: status %s, want %s", id, j.Status, want)
		}
	}
}
