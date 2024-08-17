package vmhandler

import (
	"fmt"
	"sync"

	"github.com/accelira/accelira/metrics"
	"github.com/accelira/accelira/moduleloader"
	"github.com/dop251/goja"
)

func RunScript(script string, metricsChan chan<- metrics.Metrics, wg *sync.WaitGroup, config *moduleloader.Config) {
	defer wg.Done()

	vm := goja.New()
	moduleloader.SetupFSModule(vm)
	moduleloader.SetupCryptoModule(vm)
	moduleloader.SetupJsonWebTokenModule(vm)
	moduleloader.SetupConsoleModule(vm)
	module := moduleloader.InitializeModuleExport(vm)
	vm.Set("require", moduleloader.SetupRequire(config, metricsChan))

	_, err := vm.RunScript("script.js", fmt.Sprintf("(function() { %s })();", script))
	if err != nil {
		fmt.Println("Error running script:", err)
	}

	iterations := config.Iterations
	for i := 0; i < iterations; i++ {
		// Execute the default exported function
		ExecuteExportedFunction(vm, module)
	}
}

func CreateConfigVM(content string) (*goja.Runtime, *moduleloader.Config, error) {
	vm := goja.New()
	config := &moduleloader.Config{}
	moduleloader.SetupFSModule(vm)
	moduleloader.SetupCryptoModule(vm)
	moduleloader.SetupJsonWebTokenModule(vm)
	moduleloader.SetupConsoleModule(vm)
	_ = moduleloader.InitializeModuleExport(vm)

	vm.Set("require", moduleloader.SetupRequire(config, nil)) // Pass the correct arguments

	_, err := vm.RunScript("config.js", string(content))
	if err != nil {
		return nil, nil, fmt.Errorf("error running configuration script: %w", err)
	}

	return vm, config, nil
}

func ExecuteExportedFunction(vm *goja.Runtime, module *goja.Object) {
	moduleExports := module.Get("exports")

	if fn, ok := goja.AssertFunction(moduleExports); ok {
		// CommonJS style: module.exports = function() { ... }
		ExecuteFunction(vm, fn)
	} else if defaultExport := moduleExports.ToObject(vm).Get("default"); defaultExport != nil {
		if fn, ok := goja.AssertFunction(defaultExport); ok {
			// ES6 style: export default function() { ... }
			ExecuteFunction(vm, fn)
		}
	} else {
		fmt.Println("No executable export found.")
	}
}

func ExecuteFunction(vm *goja.Runtime, fn goja.Callable) {
	_, err := fn(goja.Undefined(), vm.ToValue(nil))
	if err != nil {
		fmt.Println(err)
	}
}

// Setup jsonwebtoken module for Goja
