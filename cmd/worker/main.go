package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/logging"
	"github.com/div02-afk/task-queue/pkg/registry"
	"github.com/div02-afk/task-queue/pkg/scheduler"
	"github.com/div02-afk/task-queue/pkg/worker"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	_ = godotenv.Overload()
	logging.Setup(config.GetDefaultLoggingConfig())

	ctx := context.Background()
	logger := logging.Component("worker_cmd")
	registryPath := flag.String("registry", "", "Registry Path")

	flag.Parse()

	if *registryPath == "" {
		logger.Error("registry path flag is required")
		os.Exit(1)
	}

	brokerConfig := config.GetDefaultBrokerConfig()
	workerPoolConfig := config.GetDefaultWorkerPoolConfig()

	redisClient := redis.NewClient(
		&redis.Options{
			Addr: os.Getenv("REDIS_URL"),
		},
	)

	broker := broker.RedisBroker{
		RedisClient: *redisClient,
		Config:      brokerConfig,
	}

	NewRegistry := registry.NewRegistry(ctx)

	err := NewRegistry.RegisterDirectory(*registryPath)
	if err != nil {
		logger.Error("task registration failed", "error", err, "registry_path", *registryPath)
		os.Exit(1)
	}

	workerPool := worker.CreateWorkerPool(
		workerPoolConfig,
		NewRegistry,
		&broker,
	)

	scheduler := scheduler.Scheduler{
		Broker:       &broker,
		PollInterval: 1 * time.Second,
	}

	logger.Info(
		"starting worker runtime",
		"registry_path", *registryPath,
		"redis_addr", os.Getenv("REDIS_URL"),
		"worker_concurrency", workerPoolConfig.Concurrency,
		"scheduler_poll_interval", time.Second,
	)

	cancelScheduler := scheduler.Start(ctx)
	cancelWorkers := workerPool.StartWorkers(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("shutdown signal received", "signal", sig.String())

	cancelScheduler()
	cancelWorkers()
	time.Sleep(300 * time.Millisecond)
	logger.Info("worker runtime stopped")
}
