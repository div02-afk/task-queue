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
	SortedSet       string
}

type WorkerPoolConfig struct {
	Concurrency int
	PollTimeout time.Duration
	TaskTimeout time.Duration
	RetryDelay  time.Duration
}

func GetDefaultBrokerConfig() BrokerConfig {
	return BrokerConfig{
		PendingQueue:    "task-queue:pending",
		ProcessingQueue: "task-queue:processing",
		DLQ:             "task-queue:dlq",
		SortedSet:       "task-set",
	}
}

func GetDefaultWorkerPoolConfig() WorkerPoolConfig {
	return WorkerPoolConfig{
		Concurrency: 5,
		PollTimeout: 5 * time.Second,
		TaskTimeout: 5 * time.Second,
		RetryDelay:  500 * time.Millisecond,
	}
}
