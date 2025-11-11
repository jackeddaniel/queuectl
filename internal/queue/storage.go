package queue

import (
	"fmt"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var db *gorm.DB

// InitializeDB initializes SQLite database and creates tables
func InitializeDB(dbPath string) error {
	var err error
	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate creates tables if they don't exist
	if err := db.AutoMigrate(&Job{}, &DLQJob{}); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Create indexes for better query performance
	db.Exec("CREATE INDEX IF NOT EXISTS idx_jobs_state ON jobs(state)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at)")

	return nil
}

// CreateJob creates a new job in the database
func CreateJob(job *Job) error {
	if !job.IsValid() {
		return fmt.Errorf("invalid job: missing id or command")
	}
	if job.State == "" {
		job.State = StatePending
	}
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()

	return db.Create(job).Error
}

// GetJob retrieves a job by ID
func GetJob(id string) (*Job, error) {
	var job Job
	result := db.First(&job, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("job not found")
		}
		return nil, result.Error
	}
	return &job, nil
}

// ListJobsByState retrieves all jobs with a specific state
func ListJobsByState(state string) ([]*Job, error) {
	var jobs []*Job
	result := db.Where("state = ?", state).Order("created_at ASC").Find(&jobs)
	return jobs, result.Error
}

// ListAllJobs retrieves all jobs
func ListAllJobs() ([]*Job, error) {
	var jobs []*Job
	result := db.Order("created_at DESC").Find(&jobs)
	return jobs, result.Error
}

// UpdateJobState updates the state of a job
func UpdateJobState(jobID string, newState string) error {
	return db.Model(&Job{}).
		Where("id = ?", jobID).
		Updates(map[string]interface{}{
			"state":      newState,
			"updated_at": time.Now(),
		}).Error
}

// UpdateJob updates the entire job
func UpdateJob(job *Job) error {
	job.UpdatedAt = time.Now()
	return db.Save(job).Error
}

// LockJobForProcessing locks a job for processing (prevents duplicate execution)
// Returns the locked job or error if already locked
func LockJobForProcessing(jobID string) (*Job, error) {
	var job Job

	// Start transaction for atomic operation
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Lock and fetch job
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND state = ?", jobID, StatePending).
		First(&job).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("job not found or not in pending state")
		}
		return nil, err
	}

	// Update state to processing
	if err := tx.Model(&job).
		Updates(map[string]interface{}{
			"state":      StateProcessing,
			"updated_at": time.Now(),
		}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	job.State = StateProcessing
	return &job, nil
}

// MoveJobToDLQ moves a job to the dead letter queue
func MoveJobToDLQ(jobID string, reason string) error {
	job, err := GetJob(jobID)
	if err != nil {
		return err
	}

	dlqJob := DLQJob{
		ID:           job.ID,
		Command:      job.Command,
		Attempts:     job.Attempts,
		MaxRetries:   job.MaxRetries,
		CreatedAt:    job.CreatedAt,
		MovedToDLQAt: time.Now(),
		LastError:    job.LastError,
		Reason:       reason,
	}

	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create DLQ entry
	if err := tx.Create(&dlqJob).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Delete from main queue
	if err := tx.Delete(&Job{}, "id = ?", jobID).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// GetDLQJobs retrieves all jobs from the dead letter queue
func GetDLQJobs() ([]*DLQJob, error) {
	var jobs []*DLQJob
	result := db.Order("moved_to_dlq_at DESC").Find(&jobs)
	return jobs, result.Error
}

// GetDLQJob retrieves a specific job from DLQ
func GetDLQJob(id string) (*DLQJob, error) {
	var job DLQJob
	result := db.First(&job, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("dlq job not found")
		}
		return nil, result.Error
	}
	return &job, nil
}

// RetryDLQJob moves a job from DLQ back to pending
func RetryDLQJob(jobID string) error {
	dlqJob, err := GetDLQJob(jobID)
	if err != nil {
		return err
	}

	// Create new job with reset attempts
	newJob := Job{
		ID:         dlqJob.ID,
		Command:    dlqJob.Command,
		State:      StatePending,
		Attempts:   0,
		MaxRetries: dlqJob.MaxRetries,
		CreatedAt:  dlqJob.CreatedAt,
		UpdatedAt:  time.Now(),
		LastError:  "",
	}

	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create new job entry
	if err := tx.Create(&newJob).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Delete from DLQ
	if err := tx.Delete(&DLQJob{}, "id = ?", jobID).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// GetQueueStatus returns statistics about the queue
func GetQueueStatus() (map[string]int, error) {
	status := make(map[string]int)

	var total int64
	db.Model(&Job{}).Count(&total)
	status["total"] = int(total)

	for _, state := range []string{StatePending, StateProcessing, StateCompleted, StateFailed} {
		var count int64
		db.Model(&Job{}).Where("state = ?", state).Count(&count)
		status[state] = int(count)
	}

	var dlqCount int64
	db.Model(&DLQJob{}).Count(&dlqCount)
	status["dead"] = int(dlqCount)

	return status, nil
}

// Close closes the database connection
func Close() error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}