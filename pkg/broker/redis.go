package broker

import (
	"context"
	"log"
	"time"

	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/task"
	"github.com/redis/go-redis/v9"
)

type RedisBroker struct {
	RedisClient redis.Client
	Config      *config.BrokerConfig
}

func (r *RedisBroker) Enqueue(ctx context.Context, task *task.Task) error {
	cmds, err := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, task.ID, task.ToMap())
		pipe.ZAdd(ctx, r.Config.SortedSet, redis.Z{Score: float64(time.Now().Unix()), Member: task.ID})
		pipe.LPush(ctx, r.Config.PendingQueue, task.ID)
		return nil
	})

	// check which command failed
	for i, cmd := range cmds {
		if cmd.Err() != nil {
			log.Printf("cmd[%d] %s failed: %v", i, cmd.FullName(), cmd.Err())
		}
	}
	return err
}

func (r *RedisBroker) Dequeue(ctx context.Context) (task.Task, error) {
	taskID, err := r.RedisClient.BLMove(
		ctx,
		r.Config.PendingQueue,
		r.Config.ProcessingQueue,
		"RIGHT", "LEFT",
		0,
	).Result()
	if err != nil {
		return task.Task{}, err
	}

	var getCmd *redis.MapStringStringCmd
	_, err = r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, taskID,
			"task_stage", string(task.StageProcessing),
			"updated_at", time.Now().Format(time.RFC3339),
		)
		getCmd = pipe.HGetAll(ctx, taskID)
		return nil
	})
	if err != nil {
		return task.Task{}, err
	}

	fields, err := getCmd.Result()
	if err != nil {
		return task.Task{}, err
	}

	t, err := task.FromMap(fields)
	if err != nil {
		return task.Task{}, err
	}

	return *t, nil
}

func (r *RedisBroker) Ack(ctx context.Context, taskId string) error {
	_, err := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LRem(ctx, r.Config.ProcessingQueue, 1, taskId)
		pipe.HSet(ctx, taskId,
			"task_stage", string(task.StageCompleted),
			"updated_at", time.Now().Format(time.RFC3339),
		)
		return nil
	})

	return err
}

/*
This can be improved by using a server-side lua script for better
transaction support, current impl splits the attempt check and queue
pushes into two different pipelines
*/

func (r *RedisBroker) Nack(ctx context.Context, taskId string) error {
	var attemptsCmd *redis.IntCmd
	var maxRetriesCmd *redis.StringCmd
	_, err := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		attemptsCmd = pipe.HIncrBy(ctx, taskId, "attempts", 1)
		maxRetriesCmd = pipe.HGet(ctx, taskId, "max_retries")
		pipe.HSet(ctx, taskId,
			"task_stage", string(task.StagePending),
			"updated_at", time.Now().Format(time.RFC3339),
		)
		pipe.LRem(ctx, r.Config.ProcessingQueue, 1, taskId)
		return nil
	})

	if err != nil {
		return err
	}

	attempts, err := attemptsCmd.Result() // int64
	if err != nil {
		return err
	}
	maxRetries, err := maxRetriesCmd.Int64() // int64
	if err != nil {
		return err
	}

	if attempts > maxRetries {
		_, err := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.LPush(ctx, r.Config.DLQ, taskId)
			pipe.HSet(ctx, taskId,
				"task_stage", string(task.StageFailed),
				"updated_at", time.Now().Format(time.RFC3339),
			)
			return nil
		})
		return err

	} else {
		err := r.RedisClient.LPush(ctx, r.Config.PendingQueue, taskId).Err()
		return err
	}
}
