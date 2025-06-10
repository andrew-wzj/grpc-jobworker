package jobworker

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// JobSession represents a single job command execution
type JobSession struct {
	ID         string
	Name       string
	CmdStr     string
	Cmd        *exec.Cmd
	Status     string
	ErrorMsg   string
	StartTime  time.Time
	Duration   time.Duration
	OutputChan chan string
}

// JobWorker manages multiple jobs concurrently
type JobWorker struct {
	mu   sync.Mutex
	Jobs map[string]*JobSession
}

// NewJobWorker creates a new JobWorker instance
func NewJobWorker() *JobWorker {
	return &JobWorker{
		Jobs: make(map[string]*JobSession),
	}
}

// Run starts a new job session
func (jw *JobWorker) Run(cmdStr string, name string) (string, error) {
	fullID := uuid.New().String()
	sessionID := fullID[len(fullID)-6:]
	cmd := exec.Command("bash", "-c", cmdStr)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdout: %v", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stderr: %v", err)
	}

	outputChan := make(chan string, 100)

	job := &JobSession{
		ID:         sessionID,
		Name:       name,
		CmdStr:     cmdStr,
		Cmd:        cmd,
		Status:     "Running",
		ErrorMsg:   "",
		StartTime:  time.Now(),
		OutputChan: outputChan,
	}

	// Try to parse estimated duration from "sleep N"
	if strings.HasPrefix(cmdStr, "sleep ") {
		parts := strings.Split(cmdStr, " ")
		if len(parts) == 2 {
			if secs, err := strconv.Atoi(parts[1]); err == nil {
				job.Duration = time.Duration(secs) * time.Second
			}
		}
	}

	jw.mu.Lock()
	jw.Jobs[sessionID] = job
	jw.mu.Unlock()

	err = cmd.Start()
	if err != nil {
		jw.mu.Lock()
		job.Status = "Failed"
		job.ErrorMsg = "Failed to start: " + err.Error()
		jw.mu.Unlock()
		return "", err
	}

	// 实时读取 stdout 和 stderr
	go streamOutput(stdoutPipe, outputChan, false)
	go streamOutput(stderrPipe, outputChan, true)

	go func() {
		err := cmd.Wait()

		jw.mu.Lock()
		defer jw.mu.Unlock()
		defer close(outputChan) // 关闭输出通道以通知 gRPC 客户端结束

		if job.Status == "Stopped" {
			return
		}
		if err != nil {
			job.Status = "Failed"
			job.ErrorMsg = err.Error()
		} else {
			job.Status = "Completed"
		}
	}()

	return sessionID, nil
}

// Stop attempts to kill a running job
func (jw *JobWorker) Stop(sessionID string) error {
	jw.mu.Lock()
	defer jw.mu.Unlock()

	job, exists := jw.Jobs[sessionID]
	if !exists {
		return fmt.Errorf("job not found")
	}
	if job.Status != "Running" {
		return fmt.Errorf("job is not currently running")
	}

	if job.Cmd != nil && job.Cmd.Process != nil {
		err := job.Cmd.Process.Kill()
		if err != nil && err.Error() != os.ErrProcessDone.Error() {
			job.Status = "Failed"
			job.ErrorMsg = err.Error()
			return fmt.Errorf("failed to kill process: %v", err)
		}
		job.Status = "Stopped"
	} else {
		return fmt.Errorf("no running process found")
	}

	return nil
}

// Optional: PrintAllStatuses with progress bars
func (jw *JobWorker) PrintAllStatuses() {
	jw.mu.Lock()
	defer jw.mu.Unlock()

	if len(jw.Jobs) == 0 {
		fmt.Println("🕳 No jobs running or completed yet.")
		return
	}

	for id, job := range jw.Jobs {
		bar := ""
		if job.Status == "Running" && job.Duration > 0 {
			elapsed := time.Since(job.StartTime)
			progress := float64(elapsed) / float64(job.Duration)
			bar = buildProgressBar(progress)
		} else if job.Status == "Completed" && job.Duration > 0 {
			bar = buildProgressBar(1.0)
		} else if job.Status == "Stopped" && job.Duration > 0 {
			elapsed := time.Since(job.StartTime)
			progress := float64(elapsed) / float64(job.Duration)
			if progress > 1.0 {
				progress = 1.0
			}
			bar = buildProgressBar(progress)
		}
		fmt.Printf("🧹 ID: %s | Name: %-10s | Status: %-10s | Cmd: %-15s %s\n",
			id, job.Name, job.Status, job.CmdStr, bar)
	}
}

func buildProgressBar(p float64) string {
	length := 20
	filled := int(p * float64(length))
	if filled > length {
		filled = length
	}
	return fmt.Sprintf("[%s%s] %3.0f%%", strings.Repeat("▓", filled), strings.Repeat("░", length-filled), p*100)
}

func streamOutput(pipe io.ReadCloser, outputChan chan<- string, isErr bool) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		if isErr {
			outputChan <- "[stderr] " + line
		} else {
			outputChan <- line
		}
	}
}
