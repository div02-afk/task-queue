package scheduler

import (
	"context"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
)

type Scheduler struct {
	Broker       broker.Broker
	PollInterval time.Duration
}

func (s *Scheduler) Start(parentCtx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(parentCtx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			taskIds, err := s.Broker.GetScheduledTaskIds(dctx)
			cancel()
			if err != nil {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			var counter uint
			for i := range taskIds {
				//TODO: add chunked processing
				go s.Broker.HandleScheduledTask(ctx, taskIds[i])
				counter++
				if counter%20 == 0 {
					time.Sleep(100 * time.Millisecond)
				}
			}

			time.Sleep(s.PollInterval)
		}
	}()

	return cancel
}
