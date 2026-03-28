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
		PendingQueue:    "task-queue:pending",
		ProcessingQueue: "task-queue:processing",
		FinishedQueue:   "task-queue:finished",
		DLQ:             "task-queue:dlq",
		TimeoutSet:      "task-set:timeout",
		ScheduledSet:    "task-set:scheduled",
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
