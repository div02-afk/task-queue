package registry

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	f, err := os.CreateTemp("", "task-*.go")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name()) // clean up after
	defer f.Close()

	_, err = f.Write(taskFunc.TaskFunction)
	if err != nil {
		log.Println("Error writing to temp file: ", err)
		return nil, err
	}
	log.Println("Wrote the code to a temp file")

	ensureDir(r.wasmDir)
	wasmFile, err := os.Create(r.wasmDir + "/" + taskName + ".wasm")
	if err != nil {
		log.Println("Wasm file creation failed with error: ", err)
		return nil, err
	}
	log.Println("Created temp wasm file with name: ", wasmFile.Name())
	log.Println("Getting compiler for lang: ", taskFunc.Language)
	com, err := wasmcompiler.GetWasmCompiler(taskFunc.Language)
	if err != nil {
		return nil, err
	}
	log.Println("Compiler initialized")
	compilationErr := com.Compile(f.Name(), wasmFile.Name())

	if compilationErr != nil {
		log.Println("Compilation failed with error: ", compilationErr)
		return nil, compilationErr
	}
	log.Println("Compilation successful")
	return wasmFile, nil
}

func (r *Registry) RegisterDirectory(directoryName string) error {
	dir, err := os.ReadDir(directoryName)
	if err != nil {
		return err
	}

	for i := range dir {
		if dir[i].IsDir() {
			log.Println("Skipping dir: ", dir[i].Name())
			continue
		}
		fileData, err := os.ReadFile(filepath.Join(directoryName, dir[i].Name()))
		if err != nil {
			log.Println("Error reading file: ", dir[i].Name(), "with error: ", err)
			continue
		}
		fileName := filepath.Base(dir[i].Name())
		ext := filepath.Ext(fileName)
		name := strings.TrimSuffix(fileName, ext)
		taskFunction := TaskFunc{
			TaskFunction: fileData,
			Language:     ext,
		}
		r.Register(name, taskFunction)
	}

	keys := make([]string, 0, len(r.tasks))
	for k := range r.tasks {
		keys = append(keys, k)
	}
	log.Println("registered tasks:", keys)

	return nil
}

func (r *Registry) Register(taskName string, taskFunc TaskFunc) error {
	wasmFile, err := r.saveTaskAsWasm(taskFunc, taskName)
	if err != nil {
		return err
	}
	defer wasmFile.Close()
	log.Println("Wasm file created")
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
	return nil
}

func (r *Registry) Get(name string) (*wazero.CompiledModule, error) {
	fn, ok := r.tasks[name]
	if !ok {
		// This is done to load wasm files to memory if the registry has restarted

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
		return &taskModule, nil
	}
	return &fn, nil
}
