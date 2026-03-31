package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/div02-afk/task-queue/pkg/broker"
	"github.com/div02-afk/task-queue/pkg/config"
	"github.com/div02-afk/task-queue/pkg/logging"
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

type TaskExecutionResult struct {
	ResultSize  int
	WasmLogPath string
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
	logging.Component("worker_pool").Info("starting worker pool", "worker_count", len(wp.workers))
	for i := 0; i < len(wp.workers); i++ {
		go wp.workers[i].superwiseWorker(ctx)
	}

	return cancel
}

func (w *Worker) superwiseWorker(ctx context.Context) {
	logger := logging.Component("worker").With("worker_id", w.ID)
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("worker panicked; restarting", "panic", fmt.Sprint(r))
				}
			}()

			w.start(ctx)
		}()

		select {
		case <-ctx.Done():
			logger.Info("worker stopped")
			return
		default:
		}

	}
}

func (w *Worker) start(ctx context.Context) {
	logger := logging.Component("worker").With("worker_id", w.ID)
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker loop canceled")
			return
		default:
		}
		logger.Debug("fetching task from broker")
		currentTask, err := w.broker.Dequeue(ctx)
		if err != nil {
			logger.Error("dequeue failed", "error", err)
			time.Sleep(w.RetryDelay)
			continue
		}

		taskLogger := logger.With(
			"task_id", currentTask.ID,
			"task_name", currentTask.TaskName,
			"attempt", currentTask.Attempts+1,
			"max_retries", currentTask.MaxRetries,
			"task_kind", currentTask.Kind,
		)

		startedAt := time.Now()
		taskLogger.Info("processing task")
		result, err := w.Process(ctx, currentTask)
		if err != nil {
			taskLogger.Error("task execution failed", "error", err, "duration", time.Since(startedAt))
			if nackErr := w.broker.Nack(ctx, currentTask.ID); nackErr != nil {
				taskLogger.Error("nack failed", "error", nackErr)
			}
			time.Sleep(w.RetryDelay)
			continue
		}

		err = w.broker.Ack(ctx, currentTask.ID)
		retryAck := 0
		for retryAck < 3 && err != nil {
			taskLogger.Warn("ack failed; retrying", "retry", retryAck+1, "error", err)
			err = w.broker.Ack(ctx, currentTask.ID)
			retryAck++
		}
		if err != nil {
			taskLogger.Error("ack failed after retries", "error", err)
		} else {
			taskLogger.Info(
				"task completed",
				"duration", time.Since(startedAt),
				"result_size_bytes", result.ResultSize,
				"wasm_log_path", result.WasmLogPath,
			)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (w *Worker) Process(parentCtx context.Context, currentTask *task.Task) (*TaskExecutionResult, error) {
	logger := logging.Component("worker_process").With(
		"worker_id", w.ID,
		"task_id", currentTask.ID,
		"task_name", currentTask.TaskName,
	)

	taskFuncWasm, err := w.registry.Get(currentTask.TaskName)
	if err != nil {
		return nil, errors.New("task function not found: " + currentTask.TaskName)
	}

	wasmLogSink, err := logging.NewWasmTaskLogSink(logger, currentTask.TaskName, currentTask.ID)
	if err != nil {
		return nil, fmt.Errorf("create wasm log sink: %w", err)
	}
	defer func() {
		if closeErr := wasmLogSink.Close(); closeErr != nil {
			logger.Error("closing wasm log sink failed", "error", closeErr)
		}
	}()

	config := wazero.NewModuleConfig().
		WithName(currentTask.TaskName).
		WithStdout(wasmLogSink.Stdout()).
		WithStderr(wasmLogSink.Stderr()).
		WithStdin(os.Stdin).
		WithStartFunctions("_initialize")
	mod, instantiateError := w.registry.Runtime.InstantiateModule(parentCtx, *taskFuncWasm, config)
	if instantiateError != nil {
		return nil, instantiateError
	}
	defer mod.Close(parentCtx)
	alloc := mod.ExportedFunction("alloc")
	execute := mod.ExportedFunction("execute")
	mem := mod.Memory()

	if execute == nil {
		return nil, errors.New("execute function not found in wasm module")
	} else if alloc == nil {
		return nil, errors.New("alloc function not found in wasm module")
	}

	ctx, cancel := context.WithTimeout(parentCtx, currentTask.Timeout)
	defer cancel()
	res, err := alloc.Call(ctx, uint64(len(currentTask.Payload)))
	if err != nil {
		return nil, fmt.Errorf("alloc call failed: %w", err)
	}
	ptr := uint32(res[0])
	if ptr == 0 && len(currentTask.Payload) > 0 {
		return nil, errors.New("wasm alloc returned zero pointer for non-empty payload")
	}
	if ptr != 0 {
		if ok := mem.Write(ptr, currentTask.Payload); !ok {
			return nil, errors.New("failed to write payload to wasm memory")
		}
	}

	res, err = execute.Call(ctx, uint64(ptr), uint64(len(currentTask.Payload)))
	if err != nil {
		return nil, err
	}

	resultPtr := uint32(res[0] >> 32)
	resultLen := uint32(res[0])
	resultBytes, ok := mem.Read(resultPtr, resultLen)
	if !ok {
		return nil, fmt.Errorf("failed to read result")
	}

	return &TaskExecutionResult{
		ResultSize:  len(resultBytes),
		WasmLogPath: wasmLogSink.LogPath(),
	}, nil
}
