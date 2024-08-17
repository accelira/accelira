package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/accelira/accelira/metrics"
	"github.com/accelira/accelira/moduleloader"
	"github.com/accelira/accelira/report"
	"github.com/accelira/accelira/util"
	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/cobra"
)

func runScript(script string, metricsChan chan<- metrics.Metrics, wg *sync.WaitGroup, config *moduleloader.Config) {
	defer wg.Done()

	vm := goja.New()
	setupFsModule(vm)
	setupCryptoModule(vm)
	setupJsonWebTokenModule(vm)
	setupConsoleModule(vm)
	module := initializeModule(vm)
	vm.Set("require", moduleloader.SetupRequire(config, metricsChan))

	_, err := vm.RunScript("script.js", fmt.Sprintf("(function() { %s })();", script))
	if err != nil {
		fmt.Println("Error running script:", err)
	}

	iterations := config.Iterations
	for i := 0; i < iterations; i++ {
		// Execute the default exported function
		executeExportedFunction(vm, module)
	}
}

func createConfigVM(content string) (*goja.Runtime, *moduleloader.Config, error) {
	vm := goja.New()
	config := &moduleloader.Config{}
	setupFsModule(vm)
	setupCryptoModule(vm)
	setupJsonWebTokenModule(vm)
	setupConsoleModule(vm)
	initializeModule(vm)

	vm.Set("require", moduleloader.SetupRequire(config, nil)) // Pass the correct arguments

	_, err := vm.RunScript("config.js", string(content))
	if err != nil {
		return nil, nil, fmt.Errorf("error running configuration script: %w", err)
	}

	return vm, config, nil
}

func initializeModule(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	exports := vm.NewObject()
	module.Set("exports", exports)

	vm.Set("module", module)
	vm.Set("exports", exports)
	return module
}

// executeExportedFunction determines the export style and executes the function
func executeExportedFunction(vm *goja.Runtime, module *goja.Object) {
	moduleExports := module.Get("exports")

	if fn, ok := goja.AssertFunction(moduleExports); ok {
		// CommonJS style: module.exports = function() { ... }
		executeFunction(vm, fn)
	} else if defaultExport := moduleExports.ToObject(vm).Get("default"); defaultExport != nil {
		if fn, ok := goja.AssertFunction(defaultExport); ok {
			// ES6 style: export default function() { ... }
			executeFunction(vm, fn)
		}
	} else {
		log.Println("No executable export found.")
	}
}

// executeFunction safely executes a Goja function
func executeFunction(vm *goja.Runtime, fn goja.Callable) {
	_, err := fn(goja.Undefined(), vm.ToValue(nil))
	if err != nil {
		log.Fatal(err)
	}
}

// Setup fs module for Goja
func setupFsModule(vm *goja.Runtime) {
	vm.Set("fs", map[string]interface{}{
		"readFileSync": func(filename string, encoding string) string {
			data, err := os.ReadFile(filename)
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}
			return string(data)
		},
	})
}

// Setup crypto module for Goja
func setupCryptoModule(vm *goja.Runtime) {
	crypto := vm.NewObject()

	crypto.Set("randomBytes", func(call goja.FunctionCall) goja.Value {
		size := call.Argument(0).ToInteger()
		bytes := make([]byte, size)
		_, err := rand.Read(bytes)
		if err != nil {
			return vm.ToValue(nil)
		}
		return vm.ToValue(bytes)
	})

	crypto.Set("createHash", func(call goja.FunctionCall) goja.Value {
		hash := sha256.New()
		return vm.ToValue(map[string]interface{}{
			"update": func(data string) {
				hash.Write([]byte(data))
			},
			"digest": func(encoding string) string {
				return base64Encode(hash.Sum(nil))
			},
		})
	})

	crypto.Set("createHmac", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(1).String()
		h := hmac.New(sha256.New, []byte(key))
		return vm.ToValue(map[string]interface{}{
			"update": func(data string) {
				h.Write([]byte(data))
			},
			"digest": func(encoding string) string {
				return base64Encode(h.Sum(nil))
			},
		})
	})

	vm.Set("crypto", crypto)
}

// Setup jsonwebtoken module for Goja
func setupJsonWebTokenModule(vm *goja.Runtime) {
	vm.Set("jsonwebtoken", map[string]interface{}{
		"sign": func(payload map[string]interface{}, privateKey string, options map[string]interface{}) (string, error) {
			// Validate the key
			if len(privateKey) == 0 {
				return "", fmt.Errorf("private key is empty")
			}

			// Parse the private key
			parsedKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKey))
			if err != nil {
				return "", fmt.Errorf("error parsing private key: %v", err)
			}

			// Create the token
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(payload))
			tokenString, err := token.SignedString(parsedKey)
			if err != nil {
				return "", fmt.Errorf("error signing token: %v", err)
			}
			return tokenString, nil
		},
	})
}

// Setup console module for Goja
func setupConsoleModule(vm *goja.Runtime) {
	console := vm.NewObject()

	console.Set("log", func(call goja.FunctionCall) goja.Value {
		args := make([]any, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		fmt.Println(args...)
		return nil
	})

	console.Set("error", func(call goja.FunctionCall) goja.Value {
		args := make([]any, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		fmt.Fprintln(os.Stderr, args...)
		return nil
	})

	vm.Set("console", console)
}

// Helper function to base64 encode byte slices
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "accelira",
		Short: "Accelira performance testing tool",
	}

	var runCmd = &cobra.Command{
		Use:   "run [script]",
		Short: "Run a JavaScript test script",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			scriptPath := args[0]

			// Display logo
			logo := `
+===================================+
|    _                _ _           |
|   / \   ___ ___ ___| (_)_ __ __ _ |
|  / _ \ / __/ __/ _ \ | | '__/ _` + "`" + ` ||
| / ___ \ (_| (_|  __/ | | | | (_| ||
|/_/   \_\___\___\___|_|_|_|  \__,_||
+===================================+
`
			fmt.Print(logo)

			// Build the JavaScript code
			result := api.Build(api.BuildOptions{
				EntryPoints: []string{scriptPath},
				Bundle:      true,
				Format:      api.FormatCommonJS,
				Platform:    api.PlatformNeutral,
				Target:      api.ES2015,
				External: []string{
					"Accelira/http",
					"Accelira/assert",
					"Accelira/config",
					"Accelira/group",
					"jsonwebtoken",
					"crypto",
					"fs"},
			})

			if len(result.Errors) > 0 {
				log.Fatalf("esbuild errors: %v", result.Errors)
			}

			// Get the bundled code
			code := string(result.OutputFiles[0].Contents)

			// Create and configure VM
			_, config, err := createConfigVM(code)
			if err != nil {
				fmt.Println(err)
				return
			}

			fmt.Printf("Concurrent Users: %d\n", config.ConcurrentUsers)
			fmt.Printf("Iterations: %d\n", config.Iterations)
			fmt.Printf("Ramp-up Rate: %d\n", config.RampUpRate)

			metricsChan := make(chan metrics.Metrics, 10000)
			var metricsList []metrics.Metrics
			var mu sync.Mutex
			var metricsWG sync.WaitGroup

			// Goroutine to process metrics
			go func() {
				defer metricsWG.Done()
				for metrics := range metricsChan {
					mu.Lock()
					metricsList = append(metricsList, metrics)
					mu.Unlock()
				}
			}()
			metricsWG.Add(1)

			// Run the scripts
			var wg sync.WaitGroup
			progressWidth := 40
			for i := 0; i < config.ConcurrentUsers; i++ {
				percent := (i + 1) * 100 / config.ConcurrentUsers
				filled := percent * progressWidth / 100
				bar := fmt.Sprintf("[%s%s] %d%% (Current: %d / Target: %d)",
					util.Repeat('=', filled),
					util.Repeat(' ', progressWidth-filled),
					percent,
					i+1,
					config.ConcurrentUsers,
				)

				fmt.Printf("\r%s", bar)
				os.Stdout.Sync() // Ensure the progress bar is flushed to the output
				wg.Add(1)
				go runScript(code, metricsChan, &wg, config)
				time.Sleep(time.Duration(1000/config.RampUpRate) * time.Millisecond)
			}

			wg.Wait()

			close(metricsChan)
			metricsWG.Wait()

			report.GenerateReport(metricsList)
		},
	}

	rootCmd.AddCommand(runCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
