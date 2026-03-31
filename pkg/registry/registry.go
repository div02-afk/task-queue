package registry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/div02-afk/task-queue/pkg/logging"
	wasmcompiler "github.com/div02-afk/task-queue/pkg/wasmCompiler"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type TaskFunc struct {
	TaskFunction []byte
	Language     string
}

type Registry struct {
	tasks   map[string]wazero.CompiledModule
	wasmDir string
	Runtime wazero.Runtime
	mu      sync.RWMutex
}

func NewRegistry(ctx context.Context) *Registry {
	rt := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)
	return &Registry{
		tasks:   make(map[string]wazero.CompiledModule),
		Runtime: rt,
		wasmDir: "/wasm_task_functions",
		mu:      sync.RWMutex{},
	}
}

func (r *Registry) getWasmFilePath(taskName string) string {
	return filepath.Join(r.wasmDir, taskName+".wasm")
}

func (r *Registry) loadWasm(name string) ([]byte, error) {
	path := r.getWasmFilePath(name)
	return os.ReadFile(path)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func (r *Registry) saveTaskAsWasm(taskFunc TaskFunc, taskName string) (*os.File, error) {
	logger := logging.Component("registry").With("task_name", taskName, "language", taskFunc.Language)

	f, err := os.CreateTemp("", "task-*.go")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name()) // clean up after
	defer f.Close()

	_, err = f.Write(taskFunc.TaskFunction)
	if err != nil {
		logger.Error("write temp source file failed", "error", err)
		return nil, err
	}
	logger.Info("wrote task source to temp file", "source_path", f.Name())

	if err := ensureDir(r.wasmDir); err != nil {
		logger.Error("ensure wasm directory failed", "error", err, "wasm_dir", r.wasmDir)
		return nil, err
	}
	wasmFile, err := os.Create(r.wasmDir + "/" + taskName + ".wasm")
	if err != nil {
		logger.Error("wasm file creation failed", "error", err)
		return nil, err
	}
	logger.Info("created wasm output file", "wasm_path", wasmFile.Name())

	com, err := wasmcompiler.GetWasmCompiler(taskFunc.Language)
	if err != nil {
		logger.Error("get wasm compiler failed", "error", err)
		return nil, err
	}
	logger.Info("compiler initialized")

	compilationErr := com.Compile(f.Name(), wasmFile.Name())
	if compilationErr != nil {
		logger.Error("wasm compilation failed", "error", compilationErr)
		return nil, compilationErr
	}
	logger.Info("wasm compilation successful")
	return wasmFile, nil
}

func (r *Registry) RegisterDirectory(directoryName string) error {
	logger := logging.Component("registry").With("directory", directoryName)
	dir, err := os.ReadDir(directoryName)
	if err != nil {
		return err
	}

	for i := range dir {
		if dir[i].IsDir() {
			logger.Debug("skipping subdirectory", "name", dir[i].Name())
			continue
		}
		fileData, err := os.ReadFile(filepath.Join(directoryName, dir[i].Name()))
		if err != nil {
			logger.Error("read task source failed", "name", dir[i].Name(), "error", err)
			continue
		}
		fileName := filepath.Base(dir[i].Name())
		ext := filepath.Ext(fileName)
		name := strings.TrimSuffix(fileName, ext)
		taskFunction := TaskFunc{
			TaskFunction: fileData,
			Language:     ext,
		}
		if err := r.Register(name, taskFunction); err != nil {
			logger.Error("register task failed", "task_name", name, "error", err)
			continue
		}
	}

	keys := make([]string, 0, len(r.tasks))
	for k := range r.tasks {
		keys = append(keys, k)
	}
	logger.Info("registered tasks", "task_names", keys)

	return nil
}

func (r *Registry) Register(taskName string, taskFunc TaskFunc) error {
	logger := logging.Component("registry").With("task_name", taskName)
	wasmFile, err := r.saveTaskAsWasm(taskFunc, taskName)
	if err != nil {
		return err
	}
	defer wasmFile.Close()
	logger.Info("wasm artifact ready", "wasm_path", wasmFile.Name())
	data, err := os.ReadFile(wasmFile.Name())
	if err != nil {
		return err
	}

	compiled, err := r.Runtime.CompileModule(context.Background(), data)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.tasks[taskName] = compiled
	r.mu.Unlock()
	logger.Info("task compiled module cached in memory")
	return nil
}

func (r *Registry) Get(name string) (*wazero.CompiledModule, error) {
	fn, ok := r.tasks[name]
	if !ok {
		logger := logging.Component("registry").With("task_name", name)

		wasmData, err := r.loadWasm(name)
		if err != nil {
			return nil, err
		}
		compiled, err := r.Runtime.CompileModule(context.Background(), wasmData)
		if err != nil {
			return nil, err
		}

		taskModule := compiled
		r.mu.Lock()
		r.tasks[name] = taskModule
		r.mu.Unlock()
		logger.Info("loaded wasm module from disk into cache")
		return &taskModule, nil
	}
	return &fn, nil
}
