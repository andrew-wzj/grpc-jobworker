package jobworker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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

	var sessionID string
	for {
		sessionID = generateShortID()
		if _, exists := jw.jobs[sessionID]; !exists {
			break
		}
	}

	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create log dir: %v", err)
	}

	logPath := filepath.Join(logDir, sessionID+".log")

	// 记录 worker debug 日志
	debugLog, _ := os.OpenFile(filepath.Join(logDir, "worker_debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	fmt.Fprintf(debugLog, "[Run] sessionID=%s, cmd=%s\n", sessionID, cmdStr)
	debugLog.Close()

	if err := db.InsertJob(sessionID, name, cmdStr); err != nil {
		return "", err
	}
	if err := db.UpdateJobStatus(sessionID, "Running", "", true); err != nil {
		return "", err
	}

	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	jw.jobs[sessionID] = cmd

	go func(id string, c *exec.Cmd, logPath string, cmdStr string) {
		start := time.Now()
		logFile, err := os.Create(logPath)
		if err != nil {
			// 记录日志创建失败
			debugLog, _ := os.OpenFile(filepath.Join(logDir, "worker_debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			fmt.Fprintf(debugLog, "[LogFileError] sessionID=%s, err=%v\n", id, err)
			debugLog.Close()
			return
		}
		c.Stdout = logFile
		c.Stderr = logFile

		err = c.Run()
		duration := time.Since(start).Truncate(time.Millisecond).String()
		logFile.Close()

		// 记录详细 debug 日志
		debugLog, _ := os.OpenFile(filepath.Join(logDir, "worker_debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(debugLog, "[Result] sessionID=%s, cmd=%s, err=%v\n", id, cmdStr, err)
			if exitErr, ok := err.(*exec.ExitError); ok {
				fmt.Fprintf(debugLog, "[ExitCode] sessionID=%s, code=%d\n", id, exitErr.ExitCode())
			}
		} else {
			fmt.Fprintf(debugLog, "[Result] sessionID=%s, cmd=%s, success, duration=%s\n", id, cmdStr, duration)
		}
		debugLog.Close()

		jw.mu.Lock()
		delete(jw.jobs, id)
		jw.mu.Unlock()

		if err != nil {
			db.UpdateJobStatusWithDuration(id, "Failed", err.Error(), false, duration)
		} else {
			db.UpdateJobStatusWithDuration(id, "Completed", "", false, duration)
		}
	}(sessionID, cmd, logPath, cmdStr)

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

func (jw *JobWorker) Delete(id string) error {
	jw.mu.Lock()
	defer jw.mu.Unlock()

	// 检查数据库中的运行状态
	status, _, isRunning, err := db.QueryJobStatus(id)
	if err != nil {
		return fmt.Errorf("failed to query job status: %v", err)
	}

	if isRunning {
		return fmt.Errorf("cannot delete a running job (%s); please stop it first", status)
	}

	delete(jw.jobs, id)
	return nil
}

func generateShortID() string {
	b := make([]byte, 3) // 3 bytes = 6 hex characters
	if _, err := rand.Read(b); err != nil {
		panic("cannot generate random id")
	}
	return hex.EncodeToString(b)
}
