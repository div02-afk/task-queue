package config

import (
	"os"
	"strings"
	"time"
)

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

type LoggingConfig struct {
	Level           string
	Format          string
	PersistWasmLogs bool
	MirrorWasmLogs  bool
	WasmLogDir      string
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
		Concurrency: 1,
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

func GetDefaultLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Level:           getEnv("TASK_QUEUE_LOG_LEVEL", "info"),
		Format:          getEnv("TASK_QUEUE_LOG_FORMAT", "pretty"),
		PersistWasmLogs: getEnvBool("TASK_QUEUE_SAVE_WASM_LOGS", false),
		MirrorWasmLogs:  getEnvBool("TASK_QUEUE_MIRROR_WASM_LOGS", true),
		WasmLogDir:      getEnv("TASK_QUEUE_WASM_LOG_DIR", "runtime-logs/wasm"),
	}
}

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
