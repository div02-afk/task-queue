package wasmcompiler

import (
	"fmt"
)

type WasmCompiler interface {
	Compile(srcPath string, outPath string) error
}

func GetWasmCompiler(language string) (WasmCompiler, error) {
	switch language {
	case ".go":
		return &GoWasmCompiler{}, nil
	default:
		return nil, fmt.Errorf("wasm compiler not found for language %s", language)
	}
}
