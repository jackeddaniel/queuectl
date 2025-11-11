package worker

import (
	"context"
	//"fmt"
	"log"
	"time"

	"github.com/jackeddaniel/queuectl/internal/config"
	"github.com/jackeddaniel/queuectl/internal/queue"
	"github.com/jackeddaniel/queuectl/pkg/backoff"
)

// Worker represents a background worker that processes jobs
type Worker struct {
	ID     int
	queue  queue.Queue
	config *config.Config
	stop   chan bool
}

// NewWorker creates a new worker instance
func NewWorker(id int, q queue.Queue, cfg *config.Config) *Worker {
	return &Worker{
		ID:     id,
		queue:  q,
		config: cfg,
		stop:   make(chan bool),
	}
}

// Start begins processing jobs
func (w *Worker) Start(ctx context.Context) {
	log.Printf("Worker %d started\n", w.ID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d stopping (context cancelled)\n", w.ID)
			return
		case <-w.stop:
			log.Printf("Worker %d stopping (stop signal received)\n", w.ID)
			return
		default:
			w.processNextJob(ctx)
			time.Sleep(1 * time.Second) // Brief pause between job checks
		}
	}
}

// Stop signals the worker to stop
func (w *Worker) Stop() {
	close(w.stop)
}

// processNextJob fetches and processes the next available job
func (w *Worker) processNextJob(ctx context.Context) {
	// Dequeue next job
	job, err := w.queue.Dequeue()
	if err != nil {
		log.Printf("Worker %d: error dequeuing job: %v\n", w.ID, err)
		return
	}

	if job == nil {
		// No jobs available
		return
	}

	log.Printf("Worker %d: processing job %s (command: %s)\n", w.ID, job.ID, job.Command)

	// Execute the job
	timeout := time.Duration(w.config.GetCommandTimeout()) * time.Second
	result := ExecuteCommand(ctx, job.Command, timeout)

	if result.IsSuccess() {
		// Job succeeded
		log.Printf("Worker %d: job %s completed successfully\n", w.ID, job.ID)
		if err := w.queue.MarkCompleted(job.ID, result.GetFullOutput()); err != nil {
			log.Printf("Worker %d: error marking job %s as completed: %v\n", w.ID, job.ID, err)
		}
	} else {
		// Job failed
		errMsg := result.GetErrorMessage()
		if result.StdErr != "" {
			errMsg += "\n" + result.StdErr
		}

		log.Printf("Worker %d: job %s failed: %s\n", w.ID, job.ID, errMsg)

		// Mark as failed and increment attempts
		if err := w.queue.MarkFailed(job.ID, errMsg); err != nil {
			log.Printf("Worker %d: error marking job %s as failed: %v\n", w.ID, job.ID, err)
			return
		}

		// Get updated job to check retry logic
		updatedJob, err := w.queue.GetJob(job.ID)
		if err != nil {
			log.Printf("Worker %d: error retrieving job %s: %v\n", w.ID, job.ID, err)
			return
		}

		// Check if we should retry or move to DLQ
		if updatedJob.Attempts >= updatedJob.MaxRetries {
			log.Printf("Worker %d: job %s exceeded max retries (%d/%d), moving to DLQ\n",
				w.ID, job.ID, updatedJob.Attempts, updatedJob.MaxRetries)

			if err := queue.MoveJobToDLQ(job.ID, "Max retries exceeded"); err != nil {
				log.Printf("Worker %d: error moving job %s to DLQ: %v\n", w.ID, job.ID, err)
			}
		} else {
			// Schedule retry with exponential backoff
			delay := backoff.CalculateDelay(updatedJob.Attempts, w.config.GetBackoffBase())
			log.Printf("Worker %d: job %s will be retried in %v (attempt %d/%d)\n",
				w.ID, job.ID, delay, updatedJob.Attempts, updatedJob.MaxRetries)

			// Wait for backoff period
			time.Sleep(delay)

			// Reset job to pending state for retry
			updatedJob.State = queue.StatePending
			if err := queue.UpdateJob(updatedJob); err != nil {
				log.Printf("Worker %d: error resetting job %s for retry: %v\n", w.ID, job.ID, err)
			} else {
				// Reload pending jobs in queue
				if err := w.queue.ReloadPendingJobs(); err != nil {
					log.Printf("Worker %d: error reloading pending jobs: %v\n", w.ID, err)
				}
			}
		}
	}
}