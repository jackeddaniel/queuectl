package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/jackeddaniel/queuectl/internal/config"
	"github.com/jackeddaniel/queuectl/internal/queue"
	"github.com/jackeddaniel/queuectl/internal/worker"
)

// EnqueueCmd returns the enqueue command
func EnqueueCmd(q *queue.Queue, cfg **config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enqueue <json>",
		Short: "Enqueue a new job",
		Long:  `Enqueue a new background job. Input should be valid JSON with at least a "command" field.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var job queue.Job
			if err := json.Unmarshal([]byte(args[0]), &job); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}

			// Validate command
			if job.Command == "" {
				return fmt.Errorf("command field is required")
			}

			// Generate ID if not provided
			if job.ID == "" {
				job.ID = uuid.New().String()
			}

			// Set defaults
			if job.MaxRetries == 0 {
				job.MaxRetries = (*cfg).GetMaxRetries()
			}
			job.State = queue.StatePending

			if err := (*q).Enqueue(&job); err != nil {
				return fmt.Errorf("failed to enqueue job: %w", err)
			}

			fmt.Printf("Job enqueued successfully:\n")
			fmt.Printf("  ID: %s\n", job.ID)
			fmt.Printf("  Command: %s\n", job.Command)
			fmt.Printf("  Max Retries: %d\n", job.MaxRetries)
			return nil
		},
	}

	return cmd
}

// ListCmd returns the list command
func ListCmd(q *queue.Queue) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List jobs",
		Long:  `List all jobs, optionally filtered by state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state, _ := cmd.Flags().GetString("state")

			var jobs []*queue.Job
			var err error

			if state != "" {
				jobs, err = (*q).ListByState(state)
			} else {
				jobs, err = queue.ListAllJobs()
			}

			if err != nil {
				return fmt.Errorf("failed to list jobs: %w", err)
			}

			if len(jobs) == 0 {
				fmt.Println("No jobs found")
				return nil
			}

			// Print table header
			fmt.Printf("%-36s %-40s %-12s %-8s %-10s\n", "ID", "Command", "State", "Attempts", "Max Retries")
			fmt.Println(string(make([]byte, 110)))

			// Print jobs
			for _, job := range jobs {
				cmdDisplay := job.Command
				if len(cmdDisplay) > 40 {
					cmdDisplay = cmdDisplay[:37] + "..."
				}
				fmt.Printf("%-36s %-40s %-12s %-8d %-10d\n",
					job.ID, cmdDisplay, job.State, job.Attempts, job.MaxRetries)
			}

			fmt.Printf("\nTotal: %d jobs\n", len(jobs))
			return nil
		},
	}

	cmd.Flags().StringP("state", "s", "", "Filter by state (pending, processing, completed, failed)")

	return cmd
}

// StatusCmd returns the status command
func StatusCmd(q *queue.Queue) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show queue status",
		Long:  `Display statistics about the job queue.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := (*q).GetStatus()
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}

			fmt.Println("Queue Status:")
			fmt.Println("─────────────────────────────")
			fmt.Printf("  Total Jobs:      %d\n", status.Total)
			fmt.Printf("  Pending:         %d\n", status.Pending)
			fmt.Printf("  Processing:      %d\n", status.Processing)
			fmt.Printf("  Completed:       %d\n", status.Completed)
			fmt.Printf("  Failed:          %d\n", status.Failed)
			fmt.Printf("  Dead (DLQ):      %d\n", status.Dead)
			fmt.Println("─────────────────────────────")

			return nil
		},
	}
}

// WorkerCmd returns the worker management command
func WorkerCmd(q *queue.Queue, cfg **config.Config, dbPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage workers",
		Long:  `Start and manage background workers that process jobs.`,
	}

	// worker start subcommand
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start worker(s)",
		Long:  `Start one or more background workers to process jobs from the queue.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			count, _ := cmd.Flags().GetInt("count")
			if count <= 0 {
				count = (*cfg).GetWorkerCount()
			}

			fmt.Printf("Starting %d worker(s)...\n", count)

			// Create context for cancellation
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle shutdown signals
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			// Start workers
			workers := make([]*worker.Worker, count)
			for i := 0; i < count; i++ {
				w := worker.NewWorker(i+1, *q, *cfg)
				workers[i] = w
				go w.Start(ctx)
			}

			fmt.Println("Workers started. Press Ctrl+C to stop.")

			// Wait for shutdown signal
			<-sigChan
			fmt.Println("\nShutdown signal received. Stopping workers...")

			// Cancel context to stop all workers
			cancel()

			fmt.Println("All workers stopped.")
			return nil
		},
	}

	startCmd.Flags().IntP("count", "c", 1, "Number of workers to start")

	cmd.AddCommand(startCmd)

	return cmd
}

// DLQCmd returns the Dead Letter Queue management command
func DLQCmd(q *queue.Queue) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dlq",
		Short: "Manage Dead Letter Queue",
		Long:  `View and manage jobs in the Dead Letter Queue (jobs that failed all retries).`,
	}

	// dlq list subcommand
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List DLQ jobs",
		Long:  `List all jobs in the Dead Letter Queue.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobs, err := queue.GetDLQJobs()
			if err != nil {
				return fmt.Errorf("failed to list DLQ jobs: %w", err)
			}

			if len(jobs) == 0 {
				fmt.Println("No jobs in Dead Letter Queue")
				return nil
			}

			// Print table header
			fmt.Printf("%-36s %-40s %-8s %-20s\n", "ID", "Command", "Attempts", "Reason")
			fmt.Println(string(make([]byte, 110)))

			// Print jobs
			for _, job := range jobs {
				cmdDisplay := job.Command
				if len(cmdDisplay) > 40 {
					cmdDisplay = cmdDisplay[:37] + "..."
				}
				reasonDisplay := job.Reason
				if len(reasonDisplay) > 20 {
					reasonDisplay = reasonDisplay[:17] + "..."
				}
				fmt.Printf("%-36s %-40s %-8d %-20s\n",
					job.ID, cmdDisplay, job.Attempts, reasonDisplay)
			}

			fmt.Printf("\nTotal: %d jobs in DLQ\n", len(jobs))
			return nil
		},
	}

	// dlq retry subcommand
	retryCmd := &cobra.Command{
		Use:   "retry <job-id>",
		Short: "Retry a DLQ job",
		Long:  `Move a job from the Dead Letter Queue back to pending for retry.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]

			if err := queue.RetryDLQJob(jobID); err != nil {
				return fmt.Errorf("failed to retry job: %w", err)
			}

			fmt.Printf("Job %s moved back to pending queue\n", jobID)
			return nil
		},
	}

	// dlq inspect subcommand
	inspectCmd := &cobra.Command{
		Use:   "inspect <job-id>",
		Short: "Inspect a DLQ job",
		Long:  `Show detailed information about a job in the Dead Letter Queue.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]

			job, err := queue.GetDLQJob(jobID)
			if err != nil {
				return fmt.Errorf("failed to get DLQ job: %w", err)
			}

			fmt.Println("DLQ Job Details:")
			fmt.Println("─────────────────────────────")
			fmt.Printf("  ID:           %s\n", job.ID)
			fmt.Printf("  Command:      %s\n", job.Command)
			fmt.Printf("  Attempts:     %d\n", job.Attempts)
			fmt.Printf("  Max Retries:  %d\n", job.MaxRetries)
			fmt.Printf("  Created:      %s\n", job.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Moved to DLQ: %s\n", job.MovedToDLQAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Reason:       %s\n", job.Reason)
			if job.LastError != "" {
				fmt.Printf("  Last Error:\n%s\n", job.LastError)
			}
			fmt.Println("─────────────────────────────")

			return nil
		},
	}

	cmd.AddCommand(listCmd)
	cmd.AddCommand(retryCmd)
	cmd.AddCommand(inspectCmd)

	return cmd
}

// ConfigCmd returns the configuration management command
func ConfigCmd(cfg **config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `View and update queuectl configuration.`,
	}

	// config show subcommand
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Long:  `Display the current configuration settings.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Current Configuration:")
			fmt.Println("─────────────────────────────")
			fmt.Printf("  Max Retries:       %d\n", (*cfg).MaxRetries)
			fmt.Printf("  Backoff Base:      %d\n", (*cfg).BackoffBase)
			fmt.Printf("  Command Timeout:   %d seconds\n", (*cfg).CommandTimeoutSeconds)
			fmt.Printf("  Worker Count:      %d\n", (*cfg).WorkerCount)
			fmt.Printf("  DB Path:           %s\n", (*cfg).DBPath)
			fmt.Println("─────────────────────────────")
			return nil
		},
	}

	// config set subcommand
	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Update configuration",
		Long:  `Update one or more configuration settings.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			changed := false

			if cmd.Flags().Changed("max-retries") {
				val, _ := cmd.Flags().GetInt("max-retries")
				(*cfg).MaxRetries = val
				changed = true
			}

			if cmd.Flags().Changed("backoff-base") {
				val, _ := cmd.Flags().GetInt("backoff-base")
				(*cfg).BackoffBase = val
				changed = true
			}

			if cmd.Flags().Changed("timeout") {
				val, _ := cmd.Flags().GetInt("timeout")
				(*cfg).CommandTimeoutSeconds = val
				changed = true
			}

			if cmd.Flags().Changed("workers") {
				val, _ := cmd.Flags().GetInt("workers")
				(*cfg).WorkerCount = val
				changed = true
			}

			if !changed {
				return fmt.Errorf("no configuration values provided")
			}

			if err := (*cfg).SaveConfig(); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			fmt.Println("Configuration updated successfully")
			return nil
		},
	}

	setCmd.Flags().Int("max-retries", 0, "Maximum number of retries per job")
	setCmd.Flags().Int("backoff-base", 0, "Exponential backoff base (seconds)")
	setCmd.Flags().Int("timeout", 0, "Command timeout in seconds")
	setCmd.Flags().Int("workers", 0, "Default number of workers")

	cmd.AddCommand(showCmd)
	cmd.AddCommand(setCmd)

	return cmd
}