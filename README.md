# task-queue

`task-queue` is a small Go-based background job queue built around Redis. It provides a producer for enqueuing jobs, a worker process for consuming and executing them, and supporting components for scheduling and timeout recovery.

This repository is under active development and should be treated as a learning exercise rather than a production-ready system. The architecture, APIs, task model, and operational details are expected to change significantly as the project evolves.

## What this project is

At a high level, the project models a task pipeline like this:

- A client or caller submits work through the producer.
- The broker stores task state and queue metadata in Redis.
- Workers pull tasks, execute registered task handlers, and acknowledge or retry them.
- A scheduler moves due scheduled tasks into the pending queue.
- A reaper watches for timed-out work.

The current architecture diagram lives in [arch.svg](./arch.svg).

## Architecture

![task-queue architecture](./arch.svg)

The system currently follows this flow:

1. A client submits work to the producer.
2. The producer creates a task payload and stores it through the broker.
3. Redis keeps queue state and task metadata for pending, processing, finished, scheduled, timeout, and DLQ flows.
4. Workers pull tasks from the pending queue, execute the registered handler, then `Ack` on success or `Nack` on failure.
5. The scheduler moves due scheduled tasks back into the pending queue.
6. The reaper watches timed-out work and is intended to support recovery and cleanup.

## Project modules

### Commands

- `cmd/producer`
  Enqueues a task into Redis. Today it accepts a task name and a JSON payload from the command line.

- `cmd/worker`
  Starts the worker pool and the scheduler. It currently registers a sample task named `task_1`.

- `cmd/reaper`
  Runs the reaper loop for timed-out tasks.

### Packages

- `pkg/task`
  Task model, task stages, task serialization, and task creation.

- `pkg/producer`
  Producer logic for adding immediate and scheduled tasks.

- `pkg/broker`
  Broker interface plus the Redis-backed implementation used by the project.

- `pkg/worker`
  Worker and worker-pool logic, including dequeue, execution, retry, and ack/nack handling.

- `pkg/registry`
  In-memory registry that maps task names to Go handler functions.

- `pkg/scheduler`
  Polls scheduled tasks and moves ready tasks into the pending queue.

- `pkg/reaper`
  Polls for timed-out tasks. This area is still incomplete and currently acts as a starting point for timeout recovery.

- `pkg/config`
  Default queue names, concurrency, retry, timeout, and reaper settings.

## Requirements

- Go `1.25.6` or later
- Redis `7.x` or later
- Docker and Docker Compose, if you want Redis managed locally through containers

## How to run the project

### 1. Start Redis

Using Docker Compose:

```sh
docker compose up -d redis
```

### 2. Create environment variables

Create a `.env` file from `.env.example` and set the Redis address:

```env
REDIS_URL=localhost:6379
```

### 3. Start the worker

```sh
go run ./cmd/worker
```

This process currently also starts the scheduler.

### 4. Start the reaper

In another terminal:

```sh
go run ./cmd/reaper
```

### 5. Enqueue a task

In another terminal:

```sh
go run ./cmd/producer task_1 "{\"message\":\"hello\"}"
```

Notes about the current implementation:

- The worker currently registers only one sample task: `task_1`.
- The producer CLI requires exactly two arguments: `<task_name>` and `<json_payload>`.
- Scheduling support exists in the codebase, but the CLI path is still basic and not fully exposed yet.
- Timeout reaping and dead-letter handling are still under development.

## Queue and processing model

The current code and `arch.svg` indicate Redis is used to track task state across:

- pending
- processing
- finished
- DLQ

Redis is also used for scheduled-task and timeout metadata.

## Development setup

Install dependencies with Go modules in the usual way once network access is available:

```sh
go mod download
```

Then run the commands shown above. If you add a new task type, register it in the worker registry before producing that task.

## Contributing

Contributions are welcome while the project is still taking shape. For now, standard open-source expectations apply:

- Open an issue or start a discussion before large changes.
- Keep pull requests focused and easy to review.
- Update documentation when behavior changes.
- Add tests where practical.
- Be respectful and constructive in issues, reviews, and discussions.

## License

This project is licensed under the MIT License. See [LICENSE](./LICENSE).
