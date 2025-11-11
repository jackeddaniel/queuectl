package cli

import (
	"encoding/json"
	"fmt"
	//"log"

	"github.com/google/uuid"
	"github.com/jackeddaniel/queuectl/internal/config"
	"github.com/jackeddaniel/queuectl/internal/queue"
	"github.com/spf13/cobra"
)

// EnqueueCmd returns the enqueue command
func EnqueueCmd(q queue.Queue, cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "enqueue <json>",
		Short: "Enqueue a new job",
		Long:  `Enqueue a new background job. Input should be valid JSON.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var job queue.Job
			if err := json.Unmarshal([]byte(args[0]), &job); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}

			// Generate ID if not provided
			if job.ID == "" {
				job.ID = uuid.New().String()
			}

			// Set defaults
			if job.MaxRetries == 0 {
				job.MaxRetries = cfg.GetMaxRetries()
			}
			job.State = queue.StatePending

			if err := q.Enqueue(&job); err != nil {
				return fmt.Errorf("failed to enqueue job: %w", err)
			}

			fmt.Printf("Job enqueued: %s\n", job.ID)
			return nil
		},
	}
}

// ListCmd returns the list command
func ListCmd(q queue.Queue) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List jobs",
		Long:  `List all jobs, optionally filtered by state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state, _ := cmd.Flags().GetString("state")

			var jobs []*queue.Job
			var err error

			if state != "" {
				jobs, err = q.ListByState(state)
			} else {
				jobs, err = ListAllJobs()
			}

			if err != nil {
				return fmt.Errorf("failed to list jobs: %w", err)
			}

			if len(jobs) == 0 {
				fmt.Println("No jobs found")
				return nil
			}

			// Print table header
			fmt.Printf("%-40s %-50s %-12s %-8s\n", "ID", "Command", "State", "Attempts")
			fmt.Println(string(make([]byte, 120)))

			// Print jobs
			for _, job := range jobs {
				if len(job.Command) > 50 {
					fmt.Printf("%-40s %-50s %-12s %-8d\n", job.ID, job.Command[:47]+"...", job.State, job.Attempts)
				} else {
					fmt.Printf("%-40s %-50s %-12s %-8d\n", job.ID, job.Command, job.State, job.Attempts)
				}
			}

			return nil
		},
	}
}

// Add stub for other commands...
func StatusCmd(q queue.Queue) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show queue status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := q.GetStatus()
			if err != nil {
				return err
			}
			fmt.Printf("Queue Status:\n")
			fmt.Printf("  Total:      %d\n", status.Total)
			fmt.Printf("  Pending:    %d\n", status.Pending)
			fmt.Printf("  Processing: %d\n", status.Processing)
			fmt.Printf("  Completed:  %d\n", status.Completed)
			fmt.Printf("  Failed:     %d\n", status.Failed)
			fmt.Printf("  Dead:       %d\n", status.Dead)
			return nil
		},
	}
}

// WorkerCmd, DLQCmd, ConfigCmd will be implemented similarly...
func WorkerCmd(q queue.Queue, cfg *config.Config, dbPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Manage workers",
		// Subcommands will be added here
	}
}

func DLQCmd(q queue.Queue) *cobra.Command {
	return &cobra.Command{
		Use:   "dlq",
		Short: "Manage Dead Letter Queue",
		// Subcommands will be added here
	}
}

func ConfigCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		// Subcommands will be added here
	}
}

// Helper function (you'll need to implement this properly)
func ListAllJobs() ([]*queue.Job, error) {
	return queue.ListAllJobs()
}
