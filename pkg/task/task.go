package task

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/google/uuid"
)

type TaskStage string
type TaskKind int

const (
	StagePending    TaskStage = "pending"
	StageProcessing TaskStage = "in_progress"
	StageCompleted  TaskStage = "completed"
	StageFailed     TaskStage = "failed"
)

const (
	KindImmediate TaskKind = iota
	KindScheduled
	KindCron
)

type Task struct {

	//Task Metadata
	ID          string    `json:"id"`
	TaskName    string    `json:"task_name"`
	TaskVersion int       `json:"task_version"` // Version number for the task, useful for tracking changes or updates
	TaskStage   TaskStage `json:"task_stage"`   // e.g., "pending", "in_progress", "completed", "failed"
	Kind        TaskKind  `json:"task_kind"`

	//Payload
	Payload json.RawMessage `json:"payload"`

	//Lifecycle
	CreatedAt  time.Time     `json:"created_at"`  // Timestamp when the task was created
	UpdatedAt  time.Time     `json:"updated_at"`  // Timestamp when the task was last updated
	Attempts   int           `json:"attempts"`    // Number of attempts made to process the task
	MaxRetries int           `json:"max_retries"` // Maximum number of retry attempts allowed for the task
	Timeout    time.Duration `json:"timeout"`     // Timeout in milliseconds

	//KindScheduled
	ScheduledAt time.Time `json:"scheduled_at"`

	//KindCron
	CronExpr string `json:"cron_expr"`

	NextRunAt time.Time `json:"next_run_at"`
}

type TaskRequestPayload struct {
	TaskName    string
	Payload     json.RawMessage
	Kind        TaskKind
	ScheduledAt time.Time
	CronExpr    string
	NextRunAt   time.Time
}

func NewTask(taskRequest *TaskRequestPayload, config *config.TaskConfig) *Task {
	return &Task{
		ID:          uuid.NewString(),
		TaskName:    taskRequest.TaskName,
		TaskStage:   StagePending,
		Payload:     taskRequest.Payload,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Attempts:    0,
		MaxRetries:  config.MaxRetries,
		Timeout:     config.Timeout,
		ScheduledAt: taskRequest.ScheduledAt,
		Kind:        taskRequest.Kind,
		CronExpr:    taskRequest.CronExpr,
		NextRunAt:   taskRequest.NextRunAt,
	}
}

func (t *Task) ToMap() map[string]any {
	return map[string]any{
		"id":           t.ID,
		"task_name":    t.TaskName,
		"task_version": t.TaskVersion,
		"task_stage":   string(t.TaskStage),
		"payload":      string(t.Payload),
		"task_kind":    strconv.Itoa(int(t.Kind)),
		"created_at":   t.CreatedAt.Format(time.RFC3339),
		"updated_at":   t.UpdatedAt.Format(time.RFC3339),
		"attempts":     t.Attempts,
		"max_retries":  t.MaxRetries,
		"timeout":      t.Timeout.String(),
		"scheduled_at": t.ScheduledAt.Format(time.RFC3339),
		"cron_expr":    t.CronExpr,
		"next_run_at":  t.NextRunAt.Format(time.RFC3339),
	}
}

func FromMap(fields map[string]string) (*Task, error) {
	createdAt, err := time.Parse(time.RFC3339, fields["created_at"])
	if err != nil {
		return nil, err
	}

	updatedAt, err := time.Parse(time.RFC3339, fields["updated_at"])
	if err != nil {
		return nil, err
	}

	timeout, err := time.ParseDuration(fields["timeout"])
	if err != nil {
		return nil, err
	}

	scheduledAt, err := time.Parse(time.RFC3339, fields["scheduled_at"])
	if err != nil {
		return nil, err
	}

	nextRun, err := time.Parse(time.RFC3339, fields["next_run_at"])
	if err != nil {
		return nil, err
	}

	taskKind, _ := strconv.Atoi(fields["task_kind"])
	taskVersion, _ := strconv.Atoi(fields["task_version"])
	attempts, _ := strconv.Atoi(fields["attempts"])
	maxRetries, _ := strconv.Atoi(fields["max_retries"])

	return &Task{
		ID:          fields["id"],
		TaskName:    fields["task_name"],
		TaskVersion: taskVersion,
		TaskStage:   TaskStage(fields["task_stage"]),
		Payload:     json.RawMessage(fields["payload"]),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Attempts:    attempts,
		MaxRetries:  maxRetries,
		Timeout:     timeout,
		ScheduledAt: scheduledAt,
		CronExpr:    fields["cron_expr"],
		NextRunAt:   nextRun,
		Kind:        TaskKind(taskKind),
	}, nil
}
