package task

import (
	"errors"
	"time"

	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/google/uuid"
)

type TaskStage string

const (
    StagePending    TaskStage = "pending"
    StageProcessing TaskStage = "in_progress"
    StageCompleted  TaskStage = "completed"
    StageFailed     TaskStage = "failed"
)

type Task struct {
	ID         string
	TaskName   string
	TaskVersion int // Version number for the task, useful for tracking changes or updates
	TaskStage  TaskStage // e.g., "pending", "in_progress", "completed", "failed"
	Payload    []byte
	CreatedAt  time.Time // Timestamp when the task was created
	UpdatedAt  time.Time // Timestamp when the task was last updated
	Attempts   int
	MaxRetries int
	Timeout    time.Duration // Timeout in milliseconds
}


func NewTask(name string,payload []byte,config config.TaskConfig) (Task,error) {

	if name == "" {
		err := errors.New("Invalid Name")
		return Task{},err
	}

	if len(payload) == 0 {
		err := errors.New("Invalid Payload")
		return Task{},err
	}

	task := &Task{
		ID: uuid.NewString(),
		TaskName: name,
		TaskStage: "pending",
		Payload: payload,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Attempts: 0,
		MaxRetries: config.MaxRetries,
		Timeout: config.Timeout,
	}

	return *task, nil;
}