package reaper

import (
	"context"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/logging"
)

type Reaper struct {
	broker broker.Broker
	Config *config.ReaperConfig
}

func NewReaper(broker broker.Broker, config *config.ReaperConfig) *Reaper {
	return &Reaper{
		broker: broker,
		Config: config,
	}
}

func (r *Reaper) Reap(ctx context.Context) error {
	logger := logging.Component("reaper").With("poll_interval", r.Config.PollInterval)
	for {
		select {
		case <-ctx.Done():
			logger.Info("reaper stopped")
			return nil
		default:
		}
		tasks, err := r.broker.GetTimedOutTaskIds(ctx)
		if err != nil {
			logger.Error("timed-out task fetch failed", "error", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		//TODO: move tasks to DLQ

		if len(tasks) > 0 {
			logger.Warn("timed-out tasks found", "task_ids", tasks, "count", len(tasks))
		}
		time.Sleep(r.Config.PollInterval)
	}
}
