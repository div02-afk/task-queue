package broker

import (
	"context"
	"time"

	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/task"
	"github.com/redis/go-redis/v9"
)

type RedisBroker struct {
	redisClient redis.Client
	config      config.BrokerConfig
}

func (r *RedisBroker) Enqueue(ctx context.Context, task task.Task) error {
	_, err := r.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, task.ID, task)
		pipe.ZAdd(ctx, r.config.SortedSet, redis.Z{Score: time.Hour.Seconds(), Member: task.ID})
		pipe.LPush(ctx, r.config.PendingQueue, task.ID)
		return nil
	})
	return err
}

func (r *RedisBroker) Dequeue(ctx context.Context) (task.Task, error) {
	taskID, err := r.redisClient.BLMove(
		ctx,
		r.config.PendingQueue,
		r.config.ProcessingQueue,
		"RIGHT", "LEFT",
		0,
	).Result()
	if err != nil {
		return task.Task{}, err
	}

	var getCmd *redis.MapStringStringCmd
	_, err = r.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, taskID,
			"TaskStage", string(task.StageProcessing),
			"UpdatedAt", time.Now().Unix(),
		)
		getCmd = pipe.HGetAll(ctx, taskID)
		return nil
	})
	if err != nil {
		return task.Task{}, err
	}

	var t task.Task
	if err := getCmd.Scan(&t); err != nil {
		return task.Task{}, err
	}
	return t, nil
}

func (r *RedisBroker) Ack(ctx context.Context, taskId string) error {
	_, err := r.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LRem(ctx, r.config.ProcessingQueue, 1, taskId)
		pipe.HSet(ctx, taskId,
			"TaskStage", string(task.StageCompleted),
			"UpdatedAt", time.Now().Unix(),
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
	_, err := r.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		attemptsCmd = pipe.HIncrBy(ctx, taskId, "Attempts", 1)
		maxRetriesCmd = pipe.HGet(ctx, taskId, "MaxRetries")
		pipe.HSet(ctx, taskId,
			"TaskStage", string(task.StagePending),
			"UpdatedAt", time.Now().Unix(),
		)
		pipe.LRem(ctx, r.config.ProcessingQueue, 1, taskId)
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
		_, err := r.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.LPush(ctx, r.config.DLQ, taskId)
			pipe.HSet(ctx, taskId,
				"TaskStage", string(task.StageFailed),
				"UpdatedAt", time.Now().Unix(),
			)
			return nil
		})
		return err

	} else {
		err := r.redisClient.LPush(ctx, r.config.PendingQueue, taskId).Err()
		return err
	}
}
