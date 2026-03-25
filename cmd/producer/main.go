package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/producer"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	godotenv.Load()
	ctx := context.Background()
	args := os.Args
	if len(args) != 3 {
		log.Panic("Invalid or Missing Arguments, got: ", args)
		return
	}
	args = args[1:]
	taskName := args[0]
	payload := json.RawMessage(args[1])

	brokerConfig := config.GetDefaultBrokerConfig()
	redisClient := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_URL"),
	})
	broker := broker.RedisBroker{
		RedisClient: *redisClient,
		Config:      brokerConfig,
	}
	producer := producer.Producer{
		Broker: &broker,
	}

	//Validate task

	taskId, err := producer.AddTask(ctx, taskName, payload)

	if err != nil {
		log.Panic("Task enqueue failed: ", err)
	}
	log.Printf("Task: ", taskId, " added to queue")
}
