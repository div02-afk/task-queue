package registry

import (
	"context"
	"encoding/json"
)

type TaskFunc func(ctx context.Context, payload json.RawMessage) error

type Registry struct {
	tasks map[string]TaskFunc
}

func NewRegistry() *Registry {
	return &Registry{
		tasks: make(map[string]TaskFunc),
	}
}

func (r *Registry) Register(taskName string, taskFunc TaskFunc) {
	r.tasks[taskName] = taskFunc
}

func (r *Registry) Get(name string) (TaskFunc, bool) {
	fn, ok := r.tasks[name]
	return fn, ok
}
