package jobworker

import (
    "os/exec"          // To run Linux commands
    "sync"             // To protect shared memory (Jobs map)
    "github.com/google/uuid" // To generate a unique Session ID
)

// Struct: JobSession
// One session = One job command run
type JobSession struct {
    ID     string
    CmdStr string
    Status string // "Running", "Completed", "Failed"
}

// Struct: JobWorker
// Manager to run multiple jobs safely
type JobWorker struct {
    mu    sync.Mutex
    Jobs  map[string]*JobSession
}

// Function: NewJobWorker()
// Create a new empty JobWorker
func NewJobWorker() *JobWorker {
    return &JobWorker{
        Jobs: make(map[string]*JobSession),
    }
}

// Method: Run(cmdStr string) (sessionID, error)
// Start a Linux command in background and return Session ID
func (jw *JobWorker) Run(cmdStr string) (string, error) {
    sessionID := uuid.New().String()

    job := &JobSession{
        ID:     sessionID,
        CmdStr: cmdStr,
        Status: "Running",
    }

    // Store the job first
    jw.mu.Lock()
    jw.Jobs[sessionID] = job
    jw.mu.Unlock()

    // Run in background (thread / goroutine)
    go func() {
        cmd := exec.Command("bash", "-c", cmdStr) // Use bash -c for full Linux command
        err := cmd.Run()

        jw.mu.Lock()
        defer jw.mu.Unlock()

        if err != nil {
            job.Status = "Failed"
        } else {
            job.Status = "Completed"
        }
    }()

    return sessionID, nil
}

// Method: GetStatus(sessionID string) (status string, found bool)
// Ask a job's current status
func (jw *JobWorker) GetStatus(sessionID string) (string, bool) {
    jw.mu.Lock()
    defer jw.mu.Unlock()

    job, exists := jw.Jobs[sessionID]
    if !exists {
        return "", false
    }
    return job.Status, true
}
