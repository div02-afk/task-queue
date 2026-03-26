package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"
	"log"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/registry"
	"github.com/div02-afk/task-queue/pkg/scheduler"
	"github.com/div02-afk/task-queue/pkg/worker"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func tempTaskFunc(ctx context.Context, payload json.RawMessage) error {
	println("Executing Task with payload: ", string(payload))
	time.Sleep(5*time.Second)
	return nil
}

func main() {
	print("Starting")
	godotenv.Load()
	println("Env loaded")
	brokerConfig := config.GetDefaultBrokerConfig()
	workerPoolConfig := config.GetDefaultWorkerPoolConfig()

	redisClient := redis.NewClient(
		&redis.Options{
			Addr: os.Getenv("REDIS_URL"),
		},
	)
	println("Redis Client Created")

	broker := broker.RedisBroker{
		RedisClient: *redisClient,
		Config:      brokerConfig,
	}

	registry := registry.NewRegistry()
	registry.Register("task_1", tempTaskFunc)
	println("Registry Created and Task Registered")

	workerPool := worker.CreateWorkerPool(
		workerPoolConfig,
		registry,
		&broker,
	)

	scheduler := scheduler.Scheduler{
		Broker: &broker,
		PollInterval: 1*time.Second,
	}

	log.Println("Worker Pool and Scheduler Created")
	ctx := context.Background()
	cancelScheduler  := scheduler.Start(ctx)
	cancelWorkers := workerPool.StartWorkers(ctx)
	
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	cancelScheduler()
	cancelWorkers()
	time.Sleep(300 * time.Millisecond)

}
