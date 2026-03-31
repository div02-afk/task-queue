package wasmcompiler

import (
	"log"
)

type WasmCompiler interface {
	Compile(srcPath string, outPath string) error
}

func GetWasmCompiler(language string) (WasmCompiler, error) {
	switch language {
	case ".go":
		return &GoWasmCompiler{}, nil
	default:
		log.Println("Wasm compiler not found for language: ", language)
		return nil, nil
	}
}
