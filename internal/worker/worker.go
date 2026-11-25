// Package worker runs shell commands as background jobs.
//
// Each job runs in its own process group so that Stop also terminates any
// child processes the command started. At most MaxConcurrent jobs run at
// once; extra jobs wait in the pending state.
package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/andrew-wzj/grpc-jobworker/internal/store"
)

var (
	// ErrNotFound is returned for an unknown job ID.
	ErrNotFound = store.ErrNotFound
	// ErrNotActive is returned when stopping a job that already finished.
	ErrNotActive = errors.New("job is not pending or running")
	// ErrActive is returned when deleting a job that has not finished.
	ErrActive = errors.New("job is still pending or running; stop it first")
	// ErrInvalid is returned for an empty or oversized command.
	ErrInvalid = errors.New("invalid job")
)

// MaxCmdLen is the longest command accepted, in bytes.
const MaxCmdLen = 64 << 10

// Config configures a Worker.
type Config struct {
	// DataDir holds the job database (jobs.db) and logs (logs/<id>.log).
	DataDir string
	// MaxConcurrent is the number of jobs allowed to run at once. Default 16.
	MaxConcurrent int
	// Shell runs each command as `Shell -c <cmd>`. Default /bin/sh.
	Shell string
	// StopGrace is how long Stop waits after SIGTERM before sending SIGKILL. Default 5s.
	StopGrace time.Duration
}

// Worker runs and tracks jobs. It is safe for concurrent use.
type Worker struct {
	cfg    Config
	store  *store.Store
	logDir string
	slots  chan struct{}

	mu     sync.Mutex
	active map[string]*proc // pending and running jobs
	wg     sync.WaitGroup
}

type proc struct {
	stop          chan struct{} // closed when a stop is requested
	done          chan struct{} // closed after the final status is saved
	stopRequested bool
	cmd           *exec.Cmd // set once the process has started
}

// New opens the job database in cfg.DataDir and marks jobs left over from a
// previous run as interrupted.
func New(cfg Config) (*Worker, error) {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 16
	}
	if cfg.Shell == "" {
		cfg.Shell = "/bin/sh"
	}
	if cfg.StopGrace <= 0 {
		cfg.StopGrace = 5 * time.Second
	}
	logDir := filepath.Join(cfg.DataDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "jobs.db"))
	if err != nil {
		return nil, err
	}
	if _, err := st.MarkInterrupted(time.Now()); err != nil {
		st.Close()
		return nil, fmt.Errorf("recover jobs: %w", err)
	}
	return &Worker{
		cfg:    cfg,
		store:  st,
		logDir: logDir,
		slots:  make(chan struct{}, cfg.MaxConcurrent),
		active: make(map[string]*proc),
	}, nil
}

// Run submits a command. If name is empty, the command is used as the name.
func (w *Worker) Run(name, cmd string) (store.Job, error) {
	if cmd == "" {
		return store.Job{}, fmt.Errorf("%w: cmd is required", ErrInvalid)
	}
	if len(cmd) > MaxCmdLen {
		return store.Job{}, fmt.Errorf("%w: cmd is longer than %d bytes", ErrInvalid, MaxCmdLen)
	}
	if name == "" {
		name = truncate(cmd, 60)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	id, err := w.newID()
	if err != nil {
		return store.Job{}, err
	}
	job := store.Job{
		ID:        id,
		Name:      name,
		Cmd:       cmd,
		Status:    store.StatusPending,
		LogPath:   filepath.Join(w.logDir, id+".log"),
		CreatedAt: time.Now(),
	}
	if err := w.store.Insert(job); err != nil {
		return store.Job{}, err
	}
	p := &proc{stop: make(chan struct{}), done: make(chan struct{})}
	w.active[id] = p
	w.wg.Add(1)
	go w.execute(job, p)
	return job, nil
}

// execute waits for a free slot, runs the job and records the result.
func (w *Worker) execute(job store.Job, p *proc) {
	defer w.wg.Done()
	defer func() {
		w.mu.Lock()
		delete(w.active, job.ID)
		w.mu.Unlock()
		close(p.done)
	}()

	select {
	case w.slots <- struct{}{}:
		defer func() { <-w.slots }()
	case <-p.stop:
		w.finish(&job, store.StatusStopped, 0, "stopped before it started")
		return
	}

	logFile, err := os.Create(job.LogPath)
	if err != nil {
		w.finish(&job, store.StatusFailed, -1, "create log file: "+err.Error())
		return
	}
	defer logFile.Close()

	cmd := exec.Command(w.cfg.Shell, "-c", job.Cmd)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Start under the lock so Stop either sees the process or has already
	// flagged the job before it starts.
	w.mu.Lock()
	if p.stopRequested {
		w.mu.Unlock()
		w.finish(&job, store.StatusStopped, 0, "stopped before it started")
		return
	}
	if err := cmd.Start(); err != nil {
		w.mu.Unlock()
		w.finish(&job, store.StatusFailed, -1, "start: "+err.Error())
		return
	}
	p.cmd = cmd
	job.Status = store.StatusRunning
	job.StartedAt = time.Now()
	_ = w.store.Update(job)
	w.mu.Unlock()

	waitErr := cmd.Wait()

	w.mu.Lock()
	stopped := p.stopRequested
	w.mu.Unlock()

	var exitErr *exec.ExitError
	switch {
	case stopped:
		w.finish(&job, store.StatusStopped, exitCode(cmd), "stopped by user")
	case waitErr == nil:
		w.finish(&job, store.StatusCompleted, 0, "")
	case errors.As(waitErr, &exitErr):
		w.finish(&job, store.StatusFailed, exitErr.ExitCode(), waitErr.Error())
	default:
		w.finish(&job, store.StatusFailed, -1, waitErr.Error())
	}
}

func (w *Worker) finish(job *store.Job, status store.Status, code int, msg string) {
	job.Status = status
	job.ExitCode = code
	job.Error = msg
	job.FinishedAt = time.Now()
	_ = w.store.Update(*job)
}

// Stop terminates a pending or running job and waits until it has finished.
// A running job's process group gets SIGTERM, then SIGKILL after StopGrace.
func (w *Worker) Stop(ctx context.Context, id string) (store.Job, error) {
	w.mu.Lock()
	p, ok := w.active[id]
	if !ok {
		w.mu.Unlock()
		if _, err := w.store.Get(id); err != nil {
			return store.Job{}, err
		}
		return store.Job{}, ErrNotActive
	}
	if !p.stopRequested {
		p.stopRequested = true
		close(p.stop)
		if p.cmd != nil {
			pgid := p.cmd.Process.Pid
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
			go func() {
				select {
				case <-p.done:
				case <-time.After(w.cfg.StopGrace):
					_ = syscall.Kill(-pgid, syscall.SIGKILL)
				}
			}()
		}
	}
	w.mu.Unlock()

	select {
	case <-p.done:
	case <-ctx.Done():
		return store.Job{}, ctx.Err()
	}
	return w.store.Get(id)
}

// Get returns one job.
func (w *Worker) Get(id string) (store.Job, error) { return w.store.Get(id) }

// List returns all jobs, newest first.
func (w *Worker) List() ([]store.Job, error) { return w.store.List() }

// Delete removes a finished job and its log file.
func (w *Worker) Delete(id string) error {
	w.mu.Lock()
	_, running := w.active[id]
	w.mu.Unlock()
	if running {
		return ErrActive
	}
	job, err := w.store.Get(id)
	if err != nil {
		return err
	}
	if err := os.Remove(job.LogPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove log: %w", err)
	}
	return w.store.Delete(id)
}

// FollowPollInterval is how often Follow checks a log file for new output.
var FollowPollInterval = 100 * time.Millisecond

// Follow calls send with the job's output from the beginning, then keeps
// sending new output until the job finishes or ctx is cancelled. send must
// not keep the slice after it returns.
func (w *Worker) Follow(ctx context.Context, id string, send func([]byte) error) error {
	job, err := w.store.Get(id)
	if err != nil {
		return err
	}
	done := w.doneChan(id)

	var f *os.File
	for {
		f, err = os.Open(job.LogPath)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		// No log yet: the job is pending, or it never started.
		select {
		case <-done:
			if f, err = os.Open(job.LogPath); err != nil {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(FollowPollInterval):
			continue
		}
		break
	}
	defer f.Close()

	buf := make([]byte, 32<<10)
	finished := false
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if err := send(buf[:n]); err != nil {
				return err
			}
		}
		switch {
		case err == nil:
			continue
		case !errors.Is(err, io.EOF):
			return err
		case finished:
			return nil
		}
		select {
		case <-done:
			finished = true // read once more to pick up the last output
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(FollowPollInterval):
		}
	}
}

// doneChan returns a channel that is closed when the job finishes.
func (w *Worker) doneChan(id string) <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p, ok := w.active[id]; ok {
		return p.done
	}
	closed := make(chan struct{})
	close(closed)
	return closed
}

// Shutdown stops every active job, waits for them, and closes the database.
func (w *Worker) Shutdown(ctx context.Context) error {
	w.mu.Lock()
	ids := make([]string, 0, len(w.active))
	for id := range w.active {
		ids = append(ids, id)
	}
	w.mu.Unlock()
	for _, id := range ids {
		_, _ = w.Stop(ctx, id)
	}
	w.wg.Wait()
	return w.store.Close()
}

func (w *Worker) newID() (string, error) {
	for range 10 {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		id := hex.EncodeToString(b)
		if _, err := w.store.Get(id); errors.Is(err, store.ErrNotFound) {
			return id, nil
		}
	}
	return "", errors.New("could not generate a unique job ID")
}

func exitCode(cmd *exec.Cmd) int {
	if cmd.ProcessState == nil {
		return -1
	}
	return cmd.ProcessState.ExitCode()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
