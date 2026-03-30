package wasmcompiler

type WasmCompiler interface {
	Compile(srcPath string, outPath string) error
}

func GetWasmCompiler(language string) (WasmCompiler, error) {
	switch language {
	case "go":
		return &GoWasmCompiler{}, nil
	default:
		return nil, nil
	}
}
