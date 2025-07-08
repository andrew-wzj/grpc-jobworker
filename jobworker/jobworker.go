package jobworker

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"jobworker/db"
)

type JobWorker struct {
	mu   sync.Mutex
	jobs map[string]*exec.Cmd
}

func NewJobWorker() *JobWorker {
	return &JobWorker{
		jobs: make(map[string]*exec.Cmd),
	}
}

func (jw *JobWorker) Run(name, cmdStr string) (string, error) {
	jw.mu.Lock()
	defer jw.mu.Unlock()

	sessionID := generateShortID()
	cmd := exec.Command("bash", "-c", cmdStr)

	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("❌ Failed to create log directory: %v", err)
	}

	logPath := filepath.Join(logDir, sessionID+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		log.Printf("❌ Failed to create log file: %v", err)
	} else {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer logFile.Close()
	}

	db.InsertJob(sessionID, name, cmdStr)
	db.UpdateLogPath(sessionID, logPath)
	db.UpdateJobStatus(sessionID, "Running", "", true)

	jw.jobs[sessionID] = cmd

	go func(id string, c *exec.Cmd) {
		err := c.Run()
		jw.mu.Lock()
		delete(jw.jobs, id)
		jw.mu.Unlock()

		if err != nil {
			db.UpdateJobStatus(id, "Failed", err.Error(), false)
		} else {
			db.UpdateJobStatus(id, "Completed", "", false)
		}
	}(sessionID, cmd)

	return sessionID, nil
}

func (jw *JobWorker) Stop(id string) error {
	jw.mu.Lock()
	defer jw.mu.Unlock()

	cmd, ok := jw.jobs[id]
	if !ok {
		return fmt.Errorf("job not found")
	}

	err := cmd.Process.Kill()
	if err != nil {
		return err
	}

	delete(jw.jobs, id)
	db.UpdateJobStatus(id, "Stopped", "killed by user", false)
	return nil
}

func (jw *JobWorker) List() ([]db.JobWithStatus, error) {
	jobs := db.ListJobsWithStatus()
	return jobs, nil
}

func (jw *JobWorker) ShowLog(id string) error {
	logPath, err := db.GetLogPath(id)
	if err != nil {
		return fmt.Errorf("log path not found: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("log not found: %v", err)
	}

	fmt.Println(string(content))
	return nil
}

func generateShortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:6]
}
