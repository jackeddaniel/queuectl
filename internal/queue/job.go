package queue

import "time"

// Job states
const (
	StatePending    = "pending"
	StateProcessing = "processing"
	StateCompleted  = "completed"
	StateFailed     = "failed"
	StateDead       = "dead"
)

// Job represents a background job to be executed
type Job struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	Command    string    `json:"command"`
	State      string    `json:"state"`
	Attempts   int       `json:"attempts"`
	MaxRetries int       `json:"max_retries"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	LastError  string    `json:"last_error"`
	Output     string    `json:"output"`
}

// TableName specifies the table name for GORM
func (Job) TableName() string {
	return "jobs"
}

// DLQJob represents a job in the Dead Letter Queue
type DLQJob struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Command      string    `json:"command"`
	Attempts     int       `json:"attempts"`
	MaxRetries   int       `json:"max_retries"`
	CreatedAt    time.Time `json:"created_at"`
	MovedToDLQAt time.Time `json:"moved_to_dlq_at"`
	LastError    string    `json:"last_error"`
	Reason       string    `json:"reason"`
}

// TableName specifies the table name for GORM
func (DLQJob) TableName() string {
	return "dlq_jobs"
}

// IsValid checks if a job has required fields
func (j *Job) IsValid() bool {
	return j.ID != "" && j.Command != ""
}
