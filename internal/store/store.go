// Package store persists jobs in a SQLite database.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo needed
)

// Status is the lifecycle state of a job.
type Status string

const (
	StatusPending     Status = "pending"
	StatusRunning     Status = "running"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusStopped     Status = "stopped"
	StatusInterrupted Status = "interrupted"
)

// Active reports whether the job is still pending or running.
func (s Status) Active() bool { return s == StatusPending || s == StatusRunning }

// ErrNotFound is returned when no job has the requested ID.
var ErrNotFound = errors.New("job not found")

// Job is one submitted command and its outcome.
type Job struct {
	ID         string
	Name       string
	Cmd        string
	Status     Status
	ExitCode   int
	Error      string
	LogPath    string
	CreatedAt  time.Time
	StartedAt  time.Time // zero until the job starts
	FinishedAt time.Time // zero until the job finishes
}

// Store is a SQLite-backed job table. It is safe for concurrent use.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	cmd         TEXT NOT NULL,
	status      TEXT NOT NULL,
	exit_code   INTEGER NOT NULL DEFAULT 0,
	error       TEXT NOT NULL DEFAULT '',
	log_path    TEXT NOT NULL,
	created_at  INTEGER NOT NULL,
	started_at  INTEGER NOT NULL DEFAULT 0,
	finished_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS jobs_created_at ON jobs (created_at);
`

// Open opens (or creates) the database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite allows one writer at a time; a single connection avoids SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Insert adds a new job.
func (s *Store) Insert(j Job) error {
	_, err := s.db.Exec(
		`INSERT INTO jobs (id, name, cmd, status, exit_code, error, log_path, created_at, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.Name, j.Cmd, j.Status, j.ExitCode, j.Error, j.LogPath,
		toUnix(j.CreatedAt), toUnix(j.StartedAt), toUnix(j.FinishedAt))
	return err
}

// Update saves the mutable fields of an existing job.
func (s *Store) Update(j Job) error {
	res, err := s.db.Exec(
		`UPDATE jobs SET status = ?, exit_code = ?, error = ?, started_at = ?, finished_at = ? WHERE id = ?`,
		j.Status, j.ExitCode, j.Error, toUnix(j.StartedAt), toUnix(j.FinishedAt), j.ID)
	if err != nil {
		return err
	}
	return requireOneRow(res)
}

// Get returns the job with the given ID, or ErrNotFound.
func (s *Store) Get(id string) (Job, error) {
	row := s.db.QueryRow(`SELECT `+columns+` FROM jobs WHERE id = ?`, id)
	j, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}

// List returns all jobs, newest first.
func (s *Store) List() ([]Job, error) {
	rows, err := s.db.Query(`SELECT ` + columns + ` FROM jobs ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		j, err := scan(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// Delete removes a job, or returns ErrNotFound.
func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireOneRow(res)
}

// MarkInterrupted marks every pending or running job as interrupted. It is
// called at startup, because jobs from a previous server process are gone.
func (s *Store) MarkInterrupted(now time.Time) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE jobs SET status = ?, error = ?, finished_at = ? WHERE status IN (?, ?)`,
		StatusInterrupted, "server restarted before the job finished", toUnix(now),
		StatusPending, StatusRunning)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

const columns = `id, name, cmd, status, exit_code, error, log_path, created_at, started_at, finished_at`

type scanner interface{ Scan(dest ...any) error }

func scan(r scanner) (Job, error) {
	var j Job
	var created, started, finished int64
	err := r.Scan(&j.ID, &j.Name, &j.Cmd, &j.Status, &j.ExitCode, &j.Error, &j.LogPath,
		&created, &started, &finished)
	j.CreatedAt, j.StartedAt, j.FinishedAt = fromUnix(created), fromUnix(started), fromUnix(finished)
	return j, err
}

func requireOneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Times are stored as Unix microseconds; 0 means "not set".
func toUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMicro()
}

func fromUnix(us int64) time.Time {
	if us == 0 {
		return time.Time{}
	}
	return time.UnixMicro(us)
}
