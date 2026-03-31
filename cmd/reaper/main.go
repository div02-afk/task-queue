package main

import (
	"context"
	"os"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/logging"
	"github.com/div02-afk/task-queue/pkg/reaper"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	_ = godotenv.Load()
	logging.Setup(config.GetDefaultLoggingConfig())

	ctx := context.Background()
	logger := logging.Component("reaper_cmd")
	broker := broker.RedisBroker{
		RedisClient: *redis.NewClient(
			&redis.Options{
				Addr: os.Getenv("REDIS_URL"),
			},
		),
		Config: config.GetDefaultBrokerConfig(),
	}

	reaper := reaper.NewReaper(&broker, config.GetDefaultReaperConfig())

	logger.Info("starting reaper", "redis_addr", os.Getenv("REDIS_URL"))
	if err := reaper.Reap(ctx); err != nil {
		logger.Error("reaper exited with error", "error", err)
		os.Exit(1)
	}
}
