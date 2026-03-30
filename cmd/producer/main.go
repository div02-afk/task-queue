package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/cron_helper"
	"github.com/div02-afk/task-queue/pkg/producer"
	"github.com/div02-afk/task-queue/pkg/task"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
)

func main() {
	godotenv.Load()
	ctx := context.Background()

	taskName := flag.String("task", "", "task name")
	payloadString := flag.String("payload", "", "Payload to be sent with task")
	taskKindInt := flag.Int("kind", 0, "Kind of task ie: 0:immediate, 1:scheduled, 2:cron")
	scheduledAtStr := flag.String("scheduled_at", time.Now().Format(time.RFC3339), "Task scheduled at (kind:scheduled)")
	cronExpr := flag.String("cron", "", "Cron Expression (used with kind:cron(2))")

	flag.Parse()

	scheduledAt, err := time.Parse(time.RFC3339, *scheduledAtStr)
	taskKind := task.TaskKind(*taskKindInt)
	payload := json.RawMessage(*payloadString)
	if *taskName == "" {
		panic("Invalid Task Name")
	} else if *taskKindInt < 0 || *taskKindInt > 2 {
		panic("Invalid Task Kind")
	} else if taskKind == task.KindScheduled && (err != nil || scheduledAt.Compare(time.Now()) == -1) {
		panic("Invalid ScheduledAt time")
	} else if taskKind == task.KindCron {
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		_, err := parser.Parse(*cronExpr)
		if err != nil {
			panic("Invalid Cron Expression")
		}
	}

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

	taskRequest := &task.TaskRequestPayload{
		TaskName:    *taskName,
		Payload:     payload,
		ScheduledAt: scheduledAt,
		Kind:        taskKind,
		CronExpr:    *cronExpr,
		NextRunAt:   time.Now(),
	}

	switch taskKind {
	case task.KindScheduled:
		taskRequest.NextRunAt = scheduledAt
	case task.KindCron:
		taskRequest.NextRunAt, _ = cron_helper.GetNextRunAt(*cronExpr) //Ignoring error, cronExpr validated above
	}

	//TODO: Validate task
	if taskKind == task.KindImmediate {
		taskId, err := producer.AddTask(ctx, taskRequest)
		if err != nil {
			log.Panic("Task enqueue failed: ", err)
		}
		log.Printf("Task: %v added to queue", taskId)
	} else {
		taskId, err := producer.ScheduleTask(ctx, taskRequest)
		if err != nil {
			log.Panic("Task scheduling failed: ", err)
		}
		log.Printf("Task: %v added to queue", taskId)
	}

}
