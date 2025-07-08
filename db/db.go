package db

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func Init() {
	var err error
	DB, err = sql.Open("sqlite3", "./jobrunner.db")
	if err != nil {
		log.Fatalf("❌ Failed to open database: %v", err)
	}

	createTables()
	addMissingColumns()
}

func createTables() {
	createJobsTable := `
	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		name TEXT,
		cmd TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		output_log_path TEXT
	);`

	createStatusTable := `
	CREATE TABLE IF NOT EXISTS job_status (
		job_id TEXT PRIMARY KEY,
		is_running BOOLEAN,
		status TEXT,
		error_msg TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := DB.Exec(createJobsTable); err != nil {
		log.Fatalf("❌ Failed to create jobs table: %v", err)
	}

	if _, err := DB.Exec(createStatusTable); err != nil {
		log.Fatalf("❌ Failed to create job_status table: %v", err)
	}
}

// addMissingColumns tries to add missing columns like output_log_path
func addMissingColumns() {
	_, err := DB.Exec(`ALTER TABLE jobs ADD COLUMN output_log_path TEXT`)
	if err != nil && err.Error() != "duplicate column name: output_log_path" {
		log.Printf("⚠️ Could not add output_log_path column: %v\n", err)
	}
}

// InsertJob inserts a new job into the jobs table
func InsertJob(id, name, cmd string) error {
	_, err := DB.Exec(`INSERT INTO jobs (id, name, cmd) VALUES (?, ?, ?)`, id, name, cmd)
	return err
}

// UpdateJobStatus inserts or updates job status info
func UpdateJobStatus(id, status, errorMsg string, isRunning bool) error {
	_, err := DB.Exec(`
		INSERT INTO job_status (job_id, is_running, status, error_msg, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET
			is_running=excluded.is_running,
			status=excluded.status,
			error_msg=excluded.error_msg,
			updated_at=excluded.updated_at`,
		id, isRunning, status, errorMsg, time.Now())
	return err
}

// UpdateLogPath updates the output_log_path of a job
func UpdateLogPath(jobID string, path string) error {
	_, err := DB.Exec(`UPDATE jobs SET output_log_path = ? WHERE id = ?`, path, jobID)
	return err
}

// QueryJobStatus retrieves the current status of a job by ID
func QueryJobStatus(id string) (string, string, bool, error) {
	var status, errorMsg string
	var isRunning bool
	err := DB.QueryRow(`SELECT status, error_msg, is_running FROM job_status WHERE job_id = ?`, id).Scan(&status, &errorMsg, &isRunning)
	if err != nil {
		return "", "", false, err
	}
	return status, errorMsg, isRunning, nil
}

// JobWithStatus represents combined job and status information
type JobWithStatus struct {
	ID        string
	Name      string
	Status    string
	IsRunning bool
	ErrorMsg  string
	UpdatedAt string
}

// ListJobsWithStatus returns a list of all jobs with their statuses
func ListJobsWithStatus() []JobWithStatus {
	query := `
		SELECT j.id, j.name, s.status, s.is_running, s.error_msg, s.updated_at
		FROM jobs j
		LEFT JOIN job_status s ON j.id = s.job_id
		ORDER BY s.updated_at DESC`

	rows, err := DB.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var jobs []JobWithStatus
	for rows.Next() {
		var job JobWithStatus
		err := rows.Scan(&job.ID, &job.Name, &job.Status, &job.IsRunning, &job.ErrorMsg, &job.UpdatedAt)
		if err == nil {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

// GetJob retrieves a single job and its status by ID
func GetJob(id string) (*JobWithStatus, error) {
	query := `
		SELECT j.id, j.name, s.status, s.is_running, s.error_msg, s.updated_at
		FROM jobs j
		LEFT JOIN job_status s ON j.id = s.job_id
		WHERE j.id = ?`

	var job JobWithStatus
	err := DB.QueryRow(query, id).Scan(&job.ID, &job.Name, &job.Status, &job.IsRunning, &job.ErrorMsg, &job.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// GetLogPath returns the log file path for a given job ID
func GetLogPath(jobID string) (string, error) {
	row := DB.QueryRow("SELECT output_log_path FROM jobs WHERE id = ?", jobID)
	var path string
	err := row.Scan(&path)
	if err != nil {
		return "", err
	}
	return path, nil
}
