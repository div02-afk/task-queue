package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/registry"
	"github.com/div02-afk/task-queue/pkg/task"
	"github.com/google/uuid"
	"github.com/tetratelabs/wazero"
)

type Worker struct {
	ID          string
	TaskTimeout time.Duration
	RetryDelay  time.Duration
	registry    *registry.Registry
	broker      broker.Broker
}

type WorkerPool struct {
	workers []Worker
}

func CreateWorkerPool(config *config.WorkerPoolConfig, registry *registry.Registry, broker broker.Broker) WorkerPool {
	workerPool := WorkerPool{
		workers: make([]Worker, 0),
	}
	for i := 0; i < config.Concurrency; i++ {
		worker := createWorker(registry, broker, config.TaskTimeout, config.RetryDelay)
		workerPool.workers = append(workerPool.workers, worker)
	}
	return workerPool
}

func createWorker(registry *registry.Registry, broker broker.Broker, taskTimeout time.Duration, retryDelay time.Duration) Worker {
	return Worker{
		ID:          uuid.NewString(),
		registry:    registry,
		broker:      broker,
		TaskTimeout: taskTimeout,
		RetryDelay:  retryDelay,
	}
}

func (wp *WorkerPool) StartWorkers(bgctx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(bgctx)
	for i := 0; i < len(wp.workers); i++ {
		go wp.workers[i].superwiseWorker(ctx)
	}

	return cancel
}

func (w *Worker) superwiseWorker(ctx context.Context) {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Println("Worker ", w.ID, " failed, restarting", r)
				}
			}()

			//This blocks until a worker fails
			w.start(ctx)
		}()

		select {
		case <-ctx.Done():
			return
		default:
		}

	}
}

func (w *Worker) start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		log.Println("Worker: ", w.ID, " fetching task from broker")
		task, err := w.broker.Dequeue(ctx)
		if err != nil {
			log.Println("Dequeue failed with error: ", err)
			time.Sleep(w.RetryDelay)
			continue
		}

		err = w.Process(ctx, task)
		if err != nil {
			log.Println("Task: ", task.ID, " failed with error: ", err, " retrying...")
			w.broker.Nack(ctx, task.ID)
			time.Sleep(w.RetryDelay)
			continue
		}
		err = w.broker.Ack(ctx, task.ID)
		retryAck := 0
		for retryAck < 3 && err != nil {
			log.Println("Ack failed for task: ", task.ID, " retrying... ", retryAck)
			err = w.broker.Ack(ctx, task.ID)
			retryAck++
		}
		if err != nil {
			log.Println("Ack failed for task: ", task.ID, " after 3 retries, moving to next task")
		} else {
			log.Println("Task: ", task.ID, " completed successfully")
		}

		// Sleep for a short duration before fetching the next task to prevent tight loop in case of continuous failures
		time.Sleep(100 * time.Millisecond)
	}
}

func (w *Worker) Process(parentCtx context.Context, task *task.Task) error {
	taskFuncWasm, err := w.registry.Get(task.TaskName)
	if err != nil {
		err := errors.New("Task Function not found: " + task.TaskName)
		return err
	}
	config := wazero.NewModuleConfig().
		WithName("").
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithStdin(os.Stdin).
		WithStartFunctions("_initialize")
	mod, instantiateError := w.registry.Runtime.InstantiateModule(parentCtx, *taskFuncWasm, config)
	if instantiateError != nil {
		return instantiateError
	}
	defer mod.Close(parentCtx)
	alloc := mod.ExportedFunction("alloc")
	execute := mod.ExportedFunction("execute")
	mem := mod.Memory()

	if execute == nil {
		err := errors.New("execute function not found in wasm module")
		return err
	} else if alloc == nil {
		err := errors.New("alloc function not found in wasm module")
		return err
	}

	ctx, cancel := context.WithTimeout(parentCtx, task.Timeout)
	// 1. ask wasm to allocate memory for our payload
	res, err := alloc.Call(ctx, uint64(len(task.Payload)))
	defer cancel()
	if err != nil {
		log.Println("Alloc call failed with error: ", err)
		return err
	}
	ptr := uint32(res[0])
	// 2. write payload into wasm memory at that ptr
	if ptr != 0 {
		mem.Write(ptr, task.Payload)
	}

	// 3. call execute — wasm reads payload itself via ptr+len
	res, err = execute.Call(ctx, uint64(ptr), uint64(len(task.Payload)))
	if err != nil {
		return err
	}

	// 4. unpack result ptr+len from returned uint64
	resultPtr := uint32(res[0] >> 32)
	resultLen := uint32(res[0])

	//TODO: figure out how to store results
	_, ok := mem.Read(resultPtr, resultLen)
	if !ok {
		return fmt.Errorf("failed to read result")
	}

	return nil
}
