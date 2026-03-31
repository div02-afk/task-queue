package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/registry"
	"github.com/div02-afk/task-queue/pkg/scheduler"
	"github.com/div02-afk/task-queue/pkg/worker"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	print("Starting")
	godotenv.Load()
	ctx := context.Background()
	println("Env loaded")
	registryPath := flag.String("registry", "", "Registry Path")

	flag.Parse()

	if *registryPath == "" {
		log.Panicln("Registry not found")
	}
	
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

	NewRegistry := registry.NewRegistry(ctx)

	err := NewRegistry.RegisterDirectory(*registryPath)
	if err != nil {
		log.Panicln("Task Registration failed with error: ", err)
	}
	println("Registry Created and Task Registered")

	workerPool := worker.CreateWorkerPool(
		workerPoolConfig,
		NewRegistry,
		&broker,
	)

	scheduler := scheduler.Scheduler{
		Broker:       &broker,
		PollInterval: 1 * time.Second,
	}

	log.Println("Worker Pool and Scheduler Created")
	cancelScheduler := scheduler.Start(ctx)
	cancelWorkers := workerPool.StartWorkers(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	cancelScheduler()
	cancelWorkers()
	time.Sleep(300 * time.Millisecond)

}
