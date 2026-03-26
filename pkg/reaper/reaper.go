package reaper

import (
	"context"
	"log"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
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
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		tasks, err := r.broker.GetTimedOutTaskIds(ctx)
		if err != nil {
			log.Panicln("Timed-out task fetch failed: ", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		//TODO: move tasks to DLQ

		log.Println("Timed-out tasks: ", tasks)
		time.Sleep(1 * time.Second)
	}
}
