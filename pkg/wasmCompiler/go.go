package wasmcompiler

import (
	"fmt"
	"os/exec"
)

type GoWasmCompiler struct {
}

// Compile implements [WasmCompiler].

func (c *GoWasmCompiler) Compile(srcPath string, outPath string) error {
	cmd := exec.Command("tinygo", "build",
		"-o", outPath,
		"-target", "wasi",
		srcPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tinygo compile failed: %s", out)
	}
	return nil
}
