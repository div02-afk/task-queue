package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/registry"
	"github.com/div02-afk/task-queue/pkg/scheduler"
	"github.com/div02-afk/task-queue/pkg/worker"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	print("Starting")
	godotenv.Load()
	ctx := context.Background()
	println("Env loaded")
	brokerConfig := config.GetDefaultBrokerConfig()
	workerPoolConfig := config.GetDefaultWorkerPoolConfig()

	redisClient := redis.NewClient(
		&redis.Options{
			Addr: os.Getenv("REDIS_URL"),
		},
	)
	println("Redis Client Created")

	broker := broker.RedisBroker{
		RedisClient: *redisClient,
		Config:      brokerConfig,
	}

	NewRegistry := registry.NewRegistry(ctx)

	//Sample task function
	functionText := `package main

import (
	"encoding/json"
	"unsafe"
)

var inputBuf []byte
var resultBuf []byte

func _initialize() {}
func main() {}

//export alloc
func alloc(size uint32) uint32 {
	if size == 0 {
		return 0
	}
	inputBuf = make([]byte, size)
	return uint32(uintptr(unsafe.Pointer(&inputBuf[0])))
}

//export execute
func execute(ptr uint32, length uint32) uint64 {
	input := inputBuf[:length]

	var payload struct {
		Name string ` + "`" + `json:"name"` + "`" + `
	}
	json.Unmarshal(input, &payload)

	// --- task logic here ---
	result, _ := json.Marshal(map[string]string{
		"message": "hello " + payload.Name,
	})
	// -----------------------

	return writeResult(result)
}

func readMemory(ptr uint32, length uint32) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
}

func writeResult(data []byte) uint64 {
	resultBuf = data
	ptr := uint32(uintptr(unsafe.Pointer(&resultBuf[0])))
	return (uint64(ptr) << 32) | uint64(len(resultBuf))
}`

	//Sample task
	err := NewRegistry.Register("task_1", registry.TaskFunc{
		TaskFunctionText: functionText,
		Language:         "go",
	})
	if err != nil {
		log.Panicln("Task Registration failed with error: ", err)
	}
	println("Registry Created and Task Registered")

	workerPool := worker.CreateWorkerPool(
		workerPoolConfig,
		NewRegistry,
		&broker,
	)

	scheduler := scheduler.Scheduler{
		Broker:       &broker,
		PollInterval: 1 * time.Second,
	}

	log.Println("Worker Pool and Scheduler Created")
	cancelScheduler := scheduler.Start(ctx)
	cancelWorkers := workerPool.StartWorkers(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	cancelScheduler()
	cancelWorkers()
	time.Sleep(300 * time.Millisecond)

}
