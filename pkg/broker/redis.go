package broker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/cron_helper"
	"github.com/div02-afk/task-queue/pkg/logging"
	"github.com/div02-afk/task-queue/pkg/task"
	"github.com/redis/go-redis/v9"
)

type RedisBroker struct {
	RedisClient redis.Client
	Config      *config.BrokerConfig
}

func (r *RedisBroker) taskHashKey(taskID string) string {
	return fmt.Sprintf("%s:%s", r.Config.HashKeyPrefix, taskID)
}

func logPipelineErrors(logger *slog.Logger, cmds []redis.Cmder) {
	for i, cmd := range cmds {
		if cmd.Err() != nil {
			logger.Error("redis pipeline command failed", "index", i, "command", cmd.FullName(), "error", cmd.Err())
		}
	}
}

func (r *RedisBroker) Enqueue(ctx context.Context, queuedTask *task.Task) error {
	logger := logging.Component("redis_broker").With("task_id", queuedTask.ID, "task_name", queuedTask.TaskName)
	cmds, err := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, r.taskHashKey(queuedTask.ID), queuedTask.ToMap())
		pipe.LPush(ctx, r.Config.PendingQueue, queuedTask.ID)
		return nil
	})

	logPipelineErrors(logger, cmds)
	if err == nil {
		logger.Info("task enqueued", "queue", r.Config.PendingQueue)
	}
	return err
}

func (r *RedisBroker) Dequeue(ctx context.Context) (*task.Task, error) {
	logger := logging.Component("redis_broker")
	taskID, err := r.RedisClient.BLMove(
		ctx,
		r.Config.PendingQueue,
		r.Config.ProcessingQueue,
		"RIGHT", "LEFT",
		0,
	).Result()
	if err != nil {
		return nil, err
	}

	logger = logger.With("task_id", taskID)
	var getCmd *redis.MapStringStringCmd
	_, err = r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, r.taskHashKey(taskID),
			"task_stage", string(task.StageProcessing),
			"updated_at", time.Now().Format(time.RFC3339),
		)
		getCmd = pipe.HGetAll(ctx, r.taskHashKey(taskID))
		return nil
	})
	if err != nil {
		return nil, err
	}

	fields, err := getCmd.Result()
	if err != nil {
		return nil, err
	}
	currentTask, err := task.FromMap(fields)
	if err != nil {
		return nil, err
	}

	logger = logger.With("task_name", currentTask.TaskName)
	// TODO: add better transactional ops
	err = r.RedisClient.ZAdd(
		ctx,
		r.Config.TimeoutSet,
		redis.Z{Score: float64(time.Now().Add(currentTask.Timeout).Unix()), Member: taskID},
	).Err()
	if err != nil {
		return nil, err
	}

	logger.Info("task moved to processing", "processing_queue", r.Config.ProcessingQueue, "timeout_set", r.Config.TimeoutSet)
	return currentTask, nil
}

func (r *RedisBroker) Ack(ctx context.Context, taskID string) error {
	logger := logging.Component("redis_broker").With("task_id", taskID)
	cmds, err := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LRem(ctx, r.Config.ProcessingQueue, 1, taskID)
		pipe.LPush(ctx, r.Config.FinishedQueue, taskID)
		pipe.HSet(ctx, r.taskHashKey(taskID),
			"task_stage", string(task.StageCompleted),
			"updated_at", time.Now().Format(time.RFC3339),
		)
		return nil
	})

	logPipelineErrors(logger, cmds)
	if err == nil {
		logger.Info("task acknowledged", "finished_queue", r.Config.FinishedQueue)
	}
	return err
}

/*
This can be improved by using a server-side lua script for better
transaction support, current impl splits the attempt check and queue
pushes into two different pipelines
*/
func (r *RedisBroker) Nack(ctx context.Context, taskID string) error {
	logger := logging.Component("redis_broker").With("task_id", taskID)

	var attemptsCmd *redis.IntCmd
	var maxRetriesCmd *redis.StringCmd
	_, err := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		attemptsCmd = pipe.HIncrBy(ctx, r.taskHashKey(taskID), "attempts", 1)
		maxRetriesCmd = pipe.HGet(ctx, r.taskHashKey(taskID), "max_retries")
		pipe.HSet(ctx, r.taskHashKey(taskID),
			"task_stage", string(task.StagePending),
			"updated_at", time.Now().Format(time.RFC3339),
		)
		pipe.LRem(ctx, r.Config.ProcessingQueue, 1, taskID)
		return nil
	})
	if err != nil {
		return err
	}

	attempts, err := attemptsCmd.Result()
	if err != nil {
		return err
	}
	maxRetries, err := maxRetriesCmd.Int64()
	if err != nil {
		return err
	}

	if attempts > maxRetries {
		cmds, txErr := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.LPush(ctx, r.Config.DLQ, taskID)
			pipe.HSet(ctx, r.taskHashKey(taskID),
				"task_stage", string(task.StageFailed),
				"updated_at", time.Now().Format(time.RFC3339),
			)
			return nil
		})
		logPipelineErrors(logger, cmds)
		if txErr == nil {
			logger.Warn("task moved to dlq", "attempts", attempts, "max_retries", maxRetries, "dlq", r.Config.DLQ)
		}
		return txErr
	}

	err = r.RedisClient.LPush(ctx, r.Config.PendingQueue, taskID).Err()
	if err == nil {
		logger.Warn("task returned to pending queue", "attempts", attempts, "max_retries", maxRetries, "queue", r.Config.PendingQueue)
	}
	return err
}

func (r *RedisBroker) GetTimedOutTaskIds(ctx context.Context) ([]string, error) {
	tasks, err := r.RedisClient.ZRangeArgs(ctx, redis.ZRangeArgs{
		ByScore: true,
		Start:   "0",
		Stop:    strconv.FormatFloat(float64(time.Now().Unix()), 'f', 0, 64),
		Key:     r.Config.TimeoutSet,
	}).Result()
	if err != nil {
		return []string{}, err
	}
	return tasks, nil
}

func (r *RedisBroker) Schedule(ctx context.Context, queuedTask *task.Task) error {
	logger := logging.Component("redis_broker").With("task_id", queuedTask.ID, "task_name", queuedTask.TaskName)
	cmds, err := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, r.taskHashKey(queuedTask.ID), queuedTask.ToMap())
		pipe.ZAdd(ctx, r.Config.ScheduledSet, redis.Z{Score: float64(queuedTask.NextRunAt.Unix()), Member: queuedTask.ID})
		return nil
	})

	logPipelineErrors(logger, cmds)
	if err == nil {
		logger.Info("task scheduled", "scheduled_set", r.Config.ScheduledSet, "next_run_at", queuedTask.NextRunAt.Format(time.RFC3339))
	}
	return err
}

/*
Returns taskIds for tasks scheduled within the next second
*/
func (r *RedisBroker) GetScheduledTaskIds(ctx context.Context) ([]string, error) {
	tasks, err := r.RedisClient.ZRangeArgs(ctx, redis.ZRangeArgs{
		ByScore: true,
		Stop:    strconv.FormatFloat(float64(time.Now().Unix()), 'f', 0, 64),
		Start:   "-inf",
		Key:     r.Config.ScheduledSet,
	}).Result()
	if err != nil {
		logging.Component("redis_broker").Error("fetch scheduled tasks failed", "error", err)
		return []string{}, err
	}
	return tasks, nil
}

/*
Move taskId to pendingQueue
Standard worker will pick up this task
Used by scheduled and cron tasks
*/
func (r *RedisBroker) AddToPending(ctx context.Context, taskID string) error {
	logger := logging.Component("redis_broker").With("task_id", taskID)
	cmds, err := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, r.Config.PendingQueue, taskID)
		pipe.HSet(ctx, r.taskHashKey(taskID),
			"task_stage", string(task.StagePending),
			"updated_at", time.Now().Format(time.RFC3339),
		)
		return nil
	})

	logPipelineErrors(logger, cmds)
	if err == nil {
		logger.Info("task added to pending queue", "queue", r.Config.PendingQueue)
	}
	return err
}

/*
Adds taskId to Pending Queue
Removes taskId from scheduled zset
Used for scheduled tasks (single execution)
*/
func (r *RedisBroker) HandleScheduledTask(ctx context.Context, taskID string) error {
	logger := logging.Component("redis_broker").With("task_id", taskID)

	vals, err := r.RedisClient.HMGet(ctx, r.taskHashKey(taskID), "task_kind", "cron_expr").Result()
	if err != nil {
		logger.Error("fetch scheduled task metadata failed", "error", err)
		return err
	}
	if len(vals) < 2 || vals[0] == nil {
		return fmt.Errorf("missing required metadata for task %s", taskID)
	}

	taskKindStr, ok := vals[0].(string)
	if !ok {
		return fmt.Errorf("invalid task_kind type for task %s", taskID)
	}
	taskKindInt, err := strconv.Atoi(taskKindStr)
	if err != nil {
		return fmt.Errorf("invalid task_kind value for task %s: %w", taskID, err)
	}
	taskKind := task.TaskKind(taskKindInt)

	requeue := func(extraFields map[string]string, extraOp func(redis.Pipeliner)) error {
		now := time.Now().Format(time.RFC3339)
		cmds, txErr := r.RedisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			// Workers pop from the right side of the pending list, so due
			// scheduled/cron tasks are RPUSHed to run ahead of the normal backlog.
			pipe.RPush(ctx, r.Config.PendingQueue, taskID)

			hsetArgs := make([]interface{}, 0, 4+len(extraFields)*2)
			hsetArgs = append(hsetArgs,
				"task_stage", string(task.StagePending),
				"updated_at", now,
			)
			for key, value := range extraFields {
				hsetArgs = append(hsetArgs, key, value)
			}

			pipe.HSet(ctx, r.taskHashKey(taskID), hsetArgs...)
			if extraOp != nil {
				extraOp(pipe)
			}
			return nil
		})

		logPipelineErrors(logger, cmds)
		return txErr
	}

	switch taskKind {
	case task.KindCron:
		if len(vals) < 2 || vals[1] == nil {
			return fmt.Errorf("missing cron_expr for task %s", taskID)
		}
		cronExpr, ok := vals[1].(string)
		if !ok {
			return fmt.Errorf("invalid cron_expr type for task %s", taskID)
		}

		nextRunAt, err := cron_helper.GetNextRunAt(cronExpr)
		if err != nil {
			return fmt.Errorf("invalid cron expression %s for task %s", cronExpr, taskID)
		}

		err = requeue(
			map[string]string{"next_run_at": nextRunAt.Format(time.RFC3339)},
			func(pipe redis.Pipeliner) {
				pipe.ZAdd(ctx, r.Config.ScheduledSet, redis.Z{Score: float64(nextRunAt.Unix()), Member: taskID})
			},
		)
		if err == nil {
			logger.Info("cron task promoted", "next_run_at", nextRunAt.Format(time.RFC3339))
		}
		return err

	case task.KindScheduled:
		err := requeue(nil, func(pipe redis.Pipeliner) {
			pipe.ZRem(ctx, r.Config.ScheduledSet, taskID)
		})
		if err == nil {
			logger.Info("scheduled task promoted")
		}
		return err

	default:
		return fmt.Errorf("unsupported task kind %d for task %s", taskKind, taskID)
	}
}
