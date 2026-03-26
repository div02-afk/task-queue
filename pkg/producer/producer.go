package producer

import (
	"context"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/task"
)

type Producer struct {
	Broker broker.Broker
}

/*
Accepts a validated taskName and payload
Enqueues task, returns taskId,error
*/
func (p *Producer) AddTask(ctx context.Context, taskRequest *task.TaskRequestPayload) (string, error) {

	taskConfig := config.GetDefaultTaskConfig()

	task := task.NewTask(taskRequest, taskConfig)
	err := p.Broker.Enqueue(ctx, task)

	if err != nil {
		return "", err
	}
	return task.ID, nil
}

/*
Accepts a validated taskName and payload
Schedules task, returns taskId,error
*/
func (p *Producer) ScheduleTask(ctx context.Context, taskRequest *task.TaskRequestPayload) (string, error) {

	taskConfig := config.GetDefaultTaskConfig()

	task := task.NewTask(taskRequest, taskConfig)
	err := p.Broker.Schedule(ctx, task)

	if err != nil {
		return "", err
	}
	return task.ID, nil
}
