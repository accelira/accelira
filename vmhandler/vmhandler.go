package vmhandler

import (
	"fmt"
	"sync"

	"github.com/accelira/accelira/metrics"
	"github.com/accelira/accelira/moduleloader"
	"github.com/dop251/goja"
)

func CreateConfigVM(content string) (*goja.Runtime, *moduleloader.Config, error) {
	vm := goja.New()
	config := &moduleloader.Config{}
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

// VM pool structure
type VMPool struct {
	pool chan *goja.Runtime
}

// Initialize a new VM pool
func NewVMPool(size int, config *moduleloader.Config, metricsChan chan<- metrics.Metrics) (*VMPool, error) {
	pool := make(chan *goja.Runtime, size)
	for i := 0; i < size; i++ {
		vm := goja.New()
		moduleloader.SetupConsoleModule(vm)
		moduleloader.InitializeModuleExport(vm)
		vm.Set("require", moduleloader.SetupRequire(config, metricsChan))
		pool <- vm
	}
	return &VMPool{pool: pool}, nil
}

// Get a VM from the pool
func (p *VMPool) Get() *goja.Runtime {
	return <-p.pool
}

// Return a VM to the pool
func (p *VMPool) Put(vm *goja.Runtime) {
	p.pool <- vm
}

func RunScriptWithPool(script string, metricsChan chan<- metrics.Metrics, wg *sync.WaitGroup, config *moduleloader.Config, vmPool *VMPool) {
	defer wg.Done()

	vm := vmPool.Get()
	defer vmPool.Put(vm)

	module := moduleloader.InitializeModuleExport(vm)
	_, err := vm.RunScript("script.js", fmt.Sprintf("(function() { %s })();", script))
	if err != nil {
		fmt.Println("Error running script:", err)
		return
	}

	iterations := config.Iterations

	for i := 0; i < iterations; i++ {
		ExecuteExportedFunction(vm, module)
	}

}
