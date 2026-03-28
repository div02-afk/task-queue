package config

import "time"

type TaskConfig struct {
	MaxRetries int
	Timeout    time.Duration
}

type BrokerConfig struct {
	PendingQueue    string
	ProcessingQueue string
	DLQ             string
	FinishedQueue   string
	TimeoutSet      string
	ScheduledSet    string
	HashKeyPrefix   string
}

type WorkerPoolConfig struct {
	Concurrency int
	PollTimeout time.Duration
	TaskTimeout time.Duration
	RetryDelay  time.Duration
}

type ReaperConfig struct {
	PollInterval time.Duration
}

func GetDefaultBrokerConfig() *BrokerConfig {
	return &BrokerConfig{
		PendingQueue:    "task_queue:pending",
		ProcessingQueue: "task_queue:processing",
		FinishedQueue:   "task_queue:finished",
		DLQ:             "task_queue:dlq",
		TimeoutSet:      "task_set:timeout",
		ScheduledSet:    "task_set:scheduled",
		HashKeyPrefix:   "task_metadata",
	}
}

func GetDefaultWorkerPoolConfig() *WorkerPoolConfig {
	return &WorkerPoolConfig{
		Concurrency: 20,
		PollTimeout: 5 * time.Second,
		TaskTimeout: 5 * time.Second,
		RetryDelay:  500 * time.Millisecond,
	}
}

func GetDefaultTaskConfig() *TaskConfig {
	return &TaskConfig{
		MaxRetries: 5,
		Timeout:    5 * time.Second,
	}
}

func GetDefaultReaperConfig() *ReaperConfig {
	return &ReaperConfig{
		PollInterval: 30 * time.Second,
	}
}
