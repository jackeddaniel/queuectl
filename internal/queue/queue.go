package queue

import (
	"fmt"
	"sync"
)

// Queue defines the interface for job queue operations
type Queue interface {
	Enqueue(job *Job) error
	Dequeue() (*Job, error)
	MarkProcessing(jobID string) error
	MarkCompleted(jobID string, output string) error
	MarkFailed(jobID string, errorMsg string) error
	GetStatus() (*QueueStatus, error)
	GetJob(id string) (*Job, error)
	ListByState(state string) ([]*Job, error)
	ReloadPendingJobs() error
}

// QueueStatus represents the current state of the queue
type QueueStatus struct {
	Total      int
	Pending    int
	Processing int
	Completed  int
	Failed     int
	Dead       int
}

// DefaultQueue is the default queue implementation
type DefaultQueue struct {
	pendingJobs []*Job
	mu          sync.RWMutex
}

// NewQueue creates a new queue instance
func NewQueue(dbPath string) (Queue, error) {
	q := &DefaultQueue{
		pendingJobs: make([]*Job, 0),
	}

	// Load pending jobs from database
	jobs, err := ListJobsByState(StatePending)
	if err != nil {
		return nil, fmt.Errorf("failed to load pending jobs: %w", err)
	}
	q.pendingJobs = jobs

	return q, nil
}

// Enqueue adds a job to the queue
func (q *DefaultQueue) Enqueue(job *Job) error {
	if err := CreateJob(job); err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	q.pendingJobs = append(q.pendingJobs, job)
	return nil
}

// Dequeue retrieves and locks the next job for processing
func (q *DefaultQueue) Dequeue() (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pendingJobs) == 0 {
		// Reload from database in case new jobs were added
		jobs, err := ListJobsByState(StatePending)
		if err != nil {
			return nil, err
		}
		q.pendingJobs = jobs

		if len(q.pendingJobs) == 0 {
			return nil, nil // No jobs available
		}
	}

	// Take first job (FIFO)
	jobID := q.pendingJobs[0].ID
	q.pendingJobs = q.pendingJobs[1:]

	// Lock in database
	job, err := LockJobForProcessing(jobID)
	if err != nil {
		// If lock failed, job might have been picked up by another worker
		return nil, err
	}

	return job, nil
}

// MarkProcessing marks a job as currently processing
func (q *DefaultQueue) MarkProcessing(jobID string) error {
	return UpdateJobState(jobID, StateProcessing)
}

// MarkCompleted marks a job as successfully completed
func (q *DefaultQueue) MarkCompleted(jobID string, output string) error {
	job, err := GetJob(jobID)
	if err != nil {
		return err
	}

	job.State = StateCompleted
	job.Output = output
	return UpdateJob(job)
}

// MarkFailed marks a job as failed (but may be retried)
func (q *DefaultQueue) MarkFailed(jobID string, errorMsg string) error {
	job, err := GetJob(jobID)
	if err != nil {
		return err
	}

	job.Attempts++
	job.LastError = errorMsg
	job.State = StateFailed

	return UpdateJob(job)
}

// GetStatus returns current queue statistics
func (q *DefaultQueue) GetStatus() (*QueueStatus, error) {
	statusMap, err := GetQueueStatus()
	if err != nil {
		return nil, err
	}

	return &QueueStatus{
		Total:      statusMap["total"],
		Pending:    statusMap[StatePending],
		Processing: statusMap[StateProcessing],
		Completed:  statusMap[StateCompleted],
		Failed:     statusMap[StateFailed],
		Dead:       statusMap["dead"],
	}, nil
}

// GetJob retrieves a job by ID
func (q *DefaultQueue) GetJob(id string) (*Job, error) {
	return GetJob(id)
}

// ListByState lists all jobs in a specific state
func (q *DefaultQueue) ListByState(state string) ([]*Job, error) {
	return ListJobsByState(state)
}

// ReloadPendingJobs reloads pending jobs from database
func (q *DefaultQueue) ReloadPendingJobs() error {
	jobs, err := ListJobsByState(StatePending)
	if err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	q.pendingJobs = jobs
	return nil
}