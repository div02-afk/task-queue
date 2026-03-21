package worker

import (
	"context"
	"errors"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/registry"
	"github.com/div02-afk/task-queue/pkg/task"
	"github.com/google/uuid"
)

type Worker struct {
	ID          string
	TaskTimeout time.Duration
	RetryDelay  time.Duration
	registry    *registry.Registry
	broker      broker.Broker
}

type WorkerPool struct {
	workers []Worker
}

func CreateWorkerPool(config config.WorkerPoolConfig, registry registry.Registry, broker broker.Broker) {
	workerPool := WorkerPool{
		workers: make([]Worker, 0),
	}
	for i := 0; i < config.Concurrency; i++ {
		worker := CreateWorker(registry, broker, config.TaskTimeout, config.RetryDelay)
		workerPool.workers = append(workerPool.workers, worker)
	}
}

func CreateWorker(registry registry.Registry, broker broker.Broker, taskTimeout time.Duration, retryDelay time.Duration) Worker {
	return Worker{
		ID:          uuid.NewString(),
		registry:    &registry,
		broker:      broker,
		TaskTimeout: taskTimeout,
		RetryDelay:  retryDelay,
	}
}

func (wp *WorkerPool) StartWorkers(bgctx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(bgctx)
	for i := 0; i < len(wp.workers); i++ {
		go wp.workers[i].Start(ctx)
	}

	return cancel
}

func (w *Worker) Start(ctx context.Context) {
	for {
		task, err := w.broker.Dequeue(ctx)
		if err != nil {
			time.Sleep(w.RetryDelay)
			continue
		}
		err = w.Process(ctx, task)
		if err != nil {
			w.broker.Nack(ctx, task.ID)
			time.Sleep(w.RetryDelay)
			continue
		}
		w.broker.Ack(ctx, task.ID)
	}
}

func (w *Worker) Process(parentCtx context.Context, task task.Task) error {
	taskFunc, ok := w.registry.Get(task.TaskName)

	if !ok {
		err := errors.New("Task Function not found")
		return err
	}
	ctx, cancel := context.WithTimeout(parentCtx, task.Timeout)
	defer cancel()
	err := taskFunc(ctx, task.Payload)

	if err != nil {
		return err
	}
	return nil
}
