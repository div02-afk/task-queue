package wasmcompiler

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/div02-afk/task-queue/pkg/logging"
)

type GoWasmCompiler struct {
}

// Compile implements [WasmCompiler].

func (c *GoWasmCompiler) Compile(srcPath string, outPath string) error {
	logger := logging.Component("wasm_compiler").With("language", "go", "source_path", srcPath, "output_path", outPath)
	cmd := exec.Command("tinygo", "build",
		"-o", outPath,
		"-target", "wasi",
		srcPath,
	)

	// inherit current env
	cmd.Env = append(os.Environ(),
		"TINYGOROOT="+os.Getenv("TINYGOROOT"),
	)
	
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("tinygo build failed", "error", err, "compiler_output", string(out))
		return fmt.Errorf("tinygo compile failed: %s", out)
	}
	logger.Info("tinygo build completed")
	return nil
}
