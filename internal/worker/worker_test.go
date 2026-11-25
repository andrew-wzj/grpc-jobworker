package worker

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/andrew-wzj/grpc-jobworker/internal/store"
)

func newTestWorker(t *testing.T, maxConcurrent int) *Worker {
	t.Helper()
	w, err := New(Config{DataDir: t.TempDir(), MaxConcurrent: maxConcurrent, StopGrace: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Shutdown(context.Background()) })
	return w
}

// waitFinished polls until the job is no longer pending or running.
func waitFinished(t *testing.T, w *Worker, id string) store.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err := w.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if !j.Status.Active() {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish", id)
	return store.Job{}
}

func followAll(t *testing.T, w *Worker, id string) string {
	t.Helper()
	var buf bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.Follow(ctx, id, func(b []byte) error { buf.Write(b); return nil }); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestRunCompletes(t *testing.T) {
	w := newTestWorker(t, 4)
	job, err := w.Run("", "echo hello; echo oops >&2")
	if err != nil {
		t.Fatal(err)
	}
	if job.Name != "echo hello; echo oops >&2" {
		t.Errorf("default name = %q", job.Name)
	}
	got := waitFinished(t, w, job.ID)
	if got.Status != store.StatusCompleted || got.ExitCode != 0 {
		t.Fatalf("got %s exit %d, want completed 0", got.Status, got.ExitCode)
	}
	if got.StartedAt.IsZero() || got.FinishedAt.Before(got.StartedAt) {
		t.Errorf("bad timestamps: %+v", got)
	}
	if out := followAll(t, w, job.ID); out != "hello\noops\n" {
		t.Errorf("output = %q", out)
	}
}

func TestRunFailsWithExitCode(t *testing.T) {
	w := newTestWorker(t, 4)
	job, _ := w.Run("fail", "exit 3")
	got := waitFinished(t, w, job.ID)
	if got.Status != store.StatusFailed || got.ExitCode != 3 {
		t.Fatalf("got %s exit %d, want failed 3", got.Status, got.ExitCode)
	}
}

func TestRunRejectsEmptyCommand(t *testing.T) {
	w := newTestWorker(t, 4)
	if _, err := w.Run("x", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestStopKillsProcessGroup(t *testing.T) {
	w := newTestWorker(t, 4)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	// The shell starts a background child; Stop must kill it too.
	job, _ := w.Run("tree", "sleep 30 & echo $! > "+pidFile+"; wait")

	var childPID int
	for i := 0; i < 200 && childPID == 0; i++ {
		if b, err := os.ReadFile(pidFile); err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("child never started")
	}

	got, err := w.Stop(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.StatusStopped {
		t.Fatalf("status = %s, want stopped", got.Status)
	}
	time.Sleep(50 * time.Millisecond)
	if err := syscall.Kill(childPID, 0); err == nil {
		syscall.Kill(childPID, syscall.SIGKILL)
		t.Fatal("child process survived Stop")
	}

	if _, err := w.Stop(context.Background(), job.ID); !errors.Is(err, ErrNotActive) {
		t.Fatalf("second Stop: want ErrNotActive, got %v", err)
	}
	if _, err := w.Stop(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown Stop: want ErrNotFound, got %v", err)
	}
}

func TestStopEscalatesToSIGKILL(t *testing.T) {
	w := newTestWorker(t, 4)
	w.cfg.StopGrace = 200 * time.Millisecond
	job, _ := w.Run("stubborn", "trap '' TERM; sleep 30")
	for j, _ := w.Get(job.ID); j.Status != store.StatusRunning; j, _ = w.Get(job.ID) {
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	got, err := w.Stop(context.Background(), job.ID)
	if err != nil || got.Status != store.StatusStopped {
		t.Fatalf("Stop = %s, %v", got.Status, err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("SIGKILL was not sent after the grace period")
	}
}

func TestConcurrencyLimitQueuesJobs(t *testing.T) {
	w := newTestWorker(t, 2)
	var ids []string
	for i := 0; i < 4; i++ {
		j, err := w.Run("", "sleep 2")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, j.ID)
	}
	waitForCounts(t, w, 2, 2, 2)
	for _, id := range ids {
		if j := waitFinished(t, w, id); j.Status != store.StatusCompleted {
			t.Fatalf("%s: %s", id, j.Status)
		}
	}
}

// TestManyConcurrentJobs runs 120 jobs with a limit of 100.
func TestManyConcurrentJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	w := newTestWorker(t, 100)
	var wg sync.WaitGroup
	ids := make([]string, 120)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := w.Run("", "sleep 5")
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = j.ID
		}()
	}
	wg.Wait()
	waitForCounts(t, w, 100, 100, 20)
	for _, id := range ids {
		if j := waitFinished(t, w, id); j.Status != store.StatusCompleted {
			t.Fatalf("%s: %s", id, j.Status)
		}
	}
}

func TestStopPendingJob(t *testing.T) {
	w := newTestWorker(t, 1)
	blocker, _ := w.Run("", "sleep 1")
	queued, _ := w.Run("", "echo should-not-run")
	got, err := w.Stop(context.Background(), queued.ID)
	if err != nil || got.Status != store.StatusStopped || !got.StartedAt.IsZero() {
		t.Fatalf("Stop pending = %+v, %v", got, err)
	}
	if out := followAll(t, w, queued.ID); out != "" {
		t.Errorf("stopped pending job produced output %q", out)
	}
	waitFinished(t, w, blocker.ID)
}

func TestDelete(t *testing.T) {
	w := newTestWorker(t, 4)
	running, _ := w.Run("", "sleep 30")
	if err := w.Delete(running.ID); !errors.Is(err, ErrActive) {
		t.Fatalf("delete running: want ErrActive, got %v", err)
	}
	w.Stop(context.Background(), running.ID)

	done, _ := w.Run("", "echo hi")
	j := waitFinished(t, w, done.ID)
	if err := w.Delete(done.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(j.LogPath); !os.IsNotExist(err) {
		t.Errorf("log file still exists: %v", err)
	}
	if _, err := w.Get(done.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("job still in store: %v", err)
	}
}

func TestFollowStreamsLiveOutput(t *testing.T) {
	w := newTestWorker(t, 4)
	job, _ := w.Run("", "for i in 1 2 3; do echo line $i; sleep 0.2; done")

	var mu sync.Mutex
	var chunks []string
	err := w.Follow(context.Background(), job.ID, func(b []byte) error {
		mu.Lock()
		chunks = append(chunks, string(b))
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(chunks, ""); got != "line 1\nline 2\nline 3\n" {
		t.Fatalf("output = %q", got)
	}
	if len(chunks) < 2 {
		t.Errorf("expected output in several chunks while running, got %d", len(chunks))
	}
}

func TestRestartMarksJobsInterrupted(t *testing.T) {
	dir := t.TempDir()
	w1, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	job, _ := w1.Run("", "sleep 30")
	// Simulate a crash: close the database without stopping the job.
	w1.store.Close()
	defer func() {
		w1.mu.Lock()
		if p := w1.active[job.ID]; p != nil && p.cmd != nil {
			syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		}
		w1.mu.Unlock()
	}()

	w2, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Shutdown(context.Background())
	got, _ := w2.Get(job.ID)
	if got.Status != store.StatusInterrupted {
		t.Fatalf("status after restart = %s, want interrupted", got.Status)
	}
}

// waitForCounts polls until exactly wantRunning jobs run and wantPending wait.
// Process start-up is slow on loaded CI machines, so it does not rely on a
// fixed sleep. It fails at once if more than limit jobs ever run together.
func waitForCounts(t *testing.T, w *Worker, limit, wantRunning, wantPending int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var counts map[store.Status]int
	for time.Now().Before(deadline) {
		counts = countStatuses(t, w)
		if counts[store.StatusRunning] > limit {
			t.Fatalf("%d jobs running, limit is %d", counts[store.StatusRunning], limit)
		}
		if counts[store.StatusRunning] == wantRunning && counts[store.StatusPending] == wantPending {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("want %d running + %d pending, got %v", wantRunning, wantPending, counts)
}

func countStatuses(t *testing.T, w *Worker) map[store.Status]int {
	t.Helper()
	jobs, err := w.List()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[store.Status]int{}
	for _, j := range jobs {
		counts[j.Status]++
	}
	return counts
}
