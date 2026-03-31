package scheduler

import (
	"context"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/logging"
)

type Scheduler struct {
	Broker       broker.Broker
	PollInterval time.Duration
}

func (s *Scheduler) Start(parentCtx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(parentCtx)
	logging.Component("scheduler").Info("starting scheduler", "poll_interval", s.PollInterval)
	go s.startSchedulerLoop(ctx)

	return cancel
}

func (s *Scheduler) startSchedulerLoop(ctx context.Context) {
	logger := logging.Component("scheduler")
	for {
		select {
		case <-ctx.Done():
			logger.Info("scheduler stopped")
			return
		default:
		}

		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		taskIds, err := s.Broker.GetScheduledTaskIds(dctx)
		cancel()
		if err != nil {
			logger.Error("fetch due scheduled tasks failed", "error", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if len(taskIds) > 0 {
			logger.Info("promoting scheduled tasks", "count", len(taskIds))
		}
		var counter uint
		for _, taskID := range taskIds {
			promotedTaskID := taskID
			go func(taskID string) {
				err := s.Broker.HandleScheduledTask(ctx, taskID)
				if err != nil {
					logger.Error("scheduled task promotion failed", "task_id", taskID, "error", err)
				}
			}(promotedTaskID)
			counter++
			if counter%20 == 0 {
				time.Sleep(100 * time.Millisecond)
			}
		}

		time.Sleep(s.PollInterval)
	}

}
