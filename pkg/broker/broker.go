package broker

import (
	"context"

	"github.com/div02-afk/task-queue/pkg/task"
)

type Broker interface {
	Enqueue(ctx context.Context, task *task.Task) error
	Schedule(ctx context.Context, task *task.Task) error
	HandleScheduledTask(ctx context.Context, taskId string) error
	Dequeue(ctx context.Context) (*task.Task, error)
	Ack(ctx context.Context, taskId string) error
	Nack(ctx context.Context, taskId string) error
	GetTimedOutTaskIds(ctx context.Context) ([]string,error)
	GetScheduledTaskIds(ctx context.Context) ([]string,error)
}
