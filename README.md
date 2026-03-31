# task-queue

`task-queue` is a small Go-based background job queue built around Redis. It currently supports:

- immediate tasks
- one-off scheduled tasks
- cron-style recurring tasks
- worker execution with retries and acknowledgements
- a scheduler loop for promoting due tasks
- a reaper loop for timed-out task inspection

This repository is still evolving and should be treated as a learning project rather than a production-ready queue.

## Architecture

The latest architecture diagram lives in [arch.svg](./arch.svg).

![task-queue architecture](./arch.svg)

The current runtime is organized like this:

1. A client submits a task request to `cmd/producer`.
2. The producer enriches the request with task metadata and writes it through the Redis-backed broker.
3. Redis stores each task as a hash and tracks task IDs across queue and sorted-set structures.
4. `cmd/worker` starts a worker pool and an embedded scheduler loop.
5. Workers block on the pending queue, move tasks into processing, execute registered handlers, then `Ack` or `Nack`.
6. The scheduler polls scheduled tasks and requeues due one-off and cron tasks into the pending queue.
7. `cmd/reaper` polls for expired task timeouts. Recovery and cleanup are still incomplete.

Task handlers are now compiled to WebAssembly and executed by workers through Wazero.

## Components

### Commands

- `cmd/producer`
  Accepts CLI flags, builds a task request, and either enqueues or schedules the task.

- `cmd/worker`
  Starts the worker pool, registers task handlers, and runs the scheduler loop in the same process.

- `cmd/reaper`
  Starts the timeout reaper loop.

### Packages

- `pkg/task`
  Core task model, stages, kinds, and Redis serialization helpers.

- `pkg/producer`
  Creates tasks from request payloads and sends them to the broker.

- `pkg/broker`
  Broker interface plus the Redis implementation for enqueue, dequeue, ack/nack, scheduling, and timeout lookups.

- `pkg/worker`
  Worker pool, task processing loop, retry handling, and handler dispatch.

- `pkg/registry`
  Registers task source code, compiles it to WASM, stores compiled modules in-memory, and lazy-loads persisted `.wasm` files.

- `pkg/wasmCompiler`
  Language-aware compiler adapters used by the registry to build task functions into WASM artifacts (currently TinyGo for Go source).

- `pkg/scheduler`
  Polls the scheduled set and promotes due tasks into the pending queue.

- `pkg/reaper`
  Polls the timeout set for expired tasks. The actual timeout recovery path is not finished yet.

- `pkg/config`
  Default queue names and worker/task settings.

## Redis Data Model

Each task is stored as a Redis hash keyed as `task_metadata:<taskId>`. Redis also tracks task IDs in:

- `task_queue:pending`
- `task_queue:processing`
- `task_queue:finished`
- `task_queue:dlq`
- `task_set:scheduled`
- `task_set:timeout`

Current task kinds:

- `0`: immediate
- `1`: scheduled
- `2`: cron

Current task stages:

- `pending`
- `in_progress`
- `completed`
- `failed`

## WASM Task Execution

The worker now executes task logic from WASM modules instead of calling in-process Go handler functions.

Current task function contract:

- export `alloc(size uint32) uint32`
- export `execute(ptr uint32, length uint32) uint64`
- export start function `_initialize`

Current logging behavior:

- command, broker, scheduler, reaper, registry, compiler, and worker events emit structured logs
- wasm `stdout` and `stderr` are captured per task execution in the worker
- wasm logs can be mirrored to the terminal and optionally persisted to disk

Execution flow:

1. Worker fetches task payload from Redis.
2. Worker looks up task module in the registry.
3. If needed, registry loads `<taskName>.wasm` from the wasm directory and compiles it with Wazero.
4. Worker instantiates the module with WASI enabled.
5. Worker calls `alloc`, writes payload bytes into module memory, then calls `execute`.
6. Worker reads `(resultPtr,resultLen)` packed in the returned `uint64`.

Registration flow:

1. Worker process registers tasks via `registry.Register` with source code text and language.
2. Registry writes temporary source to disk.
3. Registry compiles source to WASM (TinyGo for language `go`).
4. Registry stores the resulting module in memory and on disk for reuse.

Notes:

- The sample task registration lives in `cmd/worker` and registers `task_1` from an inline Go source string.
- Current code stores task wasm files in `/wasm_task_functions`.

## How It Runs Today

- Producer:
  Creates a task with default metadata such as retry count and timeout, then either pushes it to the pending queue or stores it in the scheduled set.

- Worker pool:
  Uses a Redis blocking move from pending to processing, executes task logic via a WASM module (`alloc` + `execute`), and then acknowledges success or requeues/fails the task on error.

- Scheduler:
  Polls the scheduled set, promotes due tasks, and for cron tasks computes the next run before re-adding them to the scheduled set.

- Reaper:
  Reads expired task IDs from the timeout set and logs them. Moving timed-out tasks to the DLQ is still a TODO in code.

## Requirements

- Go `1.25.6`
- Redis `7.x`
- TinyGo (required to compile Go task source into WASM during task registration)
- Binaryen (required by TinyGo toolchain for WASM optimization and linking)
- Docker and Docker Compose if you want a local Redis container

## Run Locally

### 1. Start Redis

```sh
docker compose up -d redis
```

### 2. Configure environment

Create `.env` from `.env.example`:

```env
REDIS_URL=localhost:6379
TASK_QUEUE_LOG_LEVEL=info
TASK_QUEUE_LOG_FORMAT=pretty
TASK_QUEUE_SAVE_WASM_LOGS=false
TASK_QUEUE_MIRROR_WASM_LOGS=true
TASK_QUEUE_WASM_LOG_DIR=runtime-logs/wasm
```

Notes:

- `TASK_QUEUE_LOG_FORMAT=pretty` is the readable default for local development.
- Set `TASK_QUEUE_LOG_FORMAT=json` if you want logs that are easier to ship to a collector.
- Set `TASK_QUEUE_SAVE_WASM_LOGS=true` to save wasm task logs on disk.
- Saved wasm logs are written under `TASK_QUEUE_WASM_LOG_DIR/<task-name>/<task-id>.log`.

### 3. Start the worker

```sh
go run ./cmd/worker -registry ./registry
```

Notes:

- This process also starts the scheduler loop.
- The worker expects a registry directory path and compiles supported task source files there into WASM at startup.

### 4. Start the reaper

In another terminal:

```sh
go run ./cmd/reaper
```

### 5. Produce tasks

Immediate task:

```sh
go run ./cmd/producer -task task_1 -payload "{\"name\":\"world\"}" -kind 0
```

Scheduled task:

```sh
go run ./cmd/producer -task task_1 -payload "{\"name\":\"later\"}" -kind 1 -scheduled_at 2026-03-29T12:00:00+05:30
```

Cron task:

```sh
go run ./cmd/producer -task task_1 -payload "{\"name\":\"repeat\"}" -kind 2 -cron "*/10 * * * * *"
```

Notes:

- Scheduled tasks require an RFC3339 timestamp in the future.
- Cron tasks use the `robfig/cron` parser with seconds enabled, so the expression is six-field.
- When wasm log persistence is enabled, each task completion log includes the saved wasm log path.

## Current Status

Implemented now:

- Redis-backed enqueue/dequeue flow
- task retries via `Nack`
- finished queue on `Ack`
- scheduled and cron task promotion
- timeout tracking via a Redis sorted set
- WASM task registration and execution via TinyGo + Wazero

Coming soon:

- timed-out task recovery and DLQ cleanup in the reaper
- stronger validation and broker-side atomicity improvements
- task result persistence and richer wasm task tooling

## License

This project is licensed under the MIT License. See [LICENSE](./LICENSE).
