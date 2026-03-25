package producer

import (
	"context"
	"encoding/json"

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
func (p *Producer) AddTask(ctx context.Context, taskName string, payload json.RawMessage) (string, error) {

	taskConfig := config.GetDefaultTaskConfig()

	task := task.NewTask(taskName, payload, taskConfig)
	err := p.Broker.Enqueue(ctx, task)

	if err != nil {
		return "", err
	}
	return task.ID, nil
}
