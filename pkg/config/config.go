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
