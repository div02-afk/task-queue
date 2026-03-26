package main

import (
	"context"
	"os"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/reaper"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	godotenv.Load()
	ctx := context.Background()
	broker := broker.RedisBroker{
		RedisClient: *redis.NewClient(
			&redis.Options{
				Addr: os.Getenv("REDIS_URL"),
			},
		),
		Config: config.GetDefaultBrokerConfig(),
	}

	reaper := reaper.NewReaper(&broker, config.GetDefaultReaperConfig())

	reaper.Reap(ctx)
}
