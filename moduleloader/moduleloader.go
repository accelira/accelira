// File: moduleloader/moduleloader.go
package moduleloader

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/accelira/accelira/httpclient"
	"github.com/accelira/accelira/metrics" // Import the new metrics package
	"github.com/accelira/accelira/util"
	"github.com/dop251/goja"
	"github.com/golang-jwt/jwt/v4"
)

type Config struct {
	Iterations      int
	RampUpRate      int
	ConcurrentUsers int
}

func createConfigModule(config *Config) map[string]interface{} {
	return map[string]interface{}{
		"setIterations":      func(iterations int) { config.Iterations = iterations },
		"setRampUpRate":      func(rate int) { config.RampUpRate = rate },
		"setConcurrentUsers": func(users int) { config.ConcurrentUsers = users },
		"getIterations":      func() int { return config.Iterations },
		"getRampUpRate":      func() int { return config.RampUpRate },
		"getConcurrentUsers": func() int { return config.ConcurrentUsers },
	}
}

func SetupRequire(config *Config, metricsChan chan<- metrics.Metrics) func(moduleName string) interface{} {
	return func(moduleName string) interface{} {
		switch moduleName {
		case "Accelira/http":
			return createHTTPModule(metricsChan)
		case "Accelira/config":
			return createConfigModule(config)
		case "Accelira/group":
			return createGroupModule(metricsChan)
		case "Accelira/assert":
			return createAssertModule()
		}
		return nil
	}
}

// createHTTPModule handles HTTP requests (GET, POST, PUT, DELETE) and sends metrics.
func createHTTPModule(metricsChan chan<- metrics.Metrics) map[string]interface{} {
	return map[string]interface{}{
		"get": func(url string) map[string]interface{} {
			resp, err := httpclient.HttpRequest(url, "GET", nil, metricsChan)
			return createResponseObject(resp, err)
		},
		"post": func(url string, body string) map[string]interface{} {
			resp, err := httpclient.HttpRequest(url, "POST", strings.NewReader(body), metricsChan)
			return createResponseObject(resp, err)
		},
		"put": func(url string, body string) map[string]interface{} {
			resp, err := httpclient.HttpRequest(url, "PUT", strings.NewReader(body), metricsChan)
			return createResponseObject(resp, err)
		},
		"delete": func(url string) map[string]interface{} {
			resp, err := httpclient.HttpRequest(url, "DELETE", nil, metricsChan)
			return createResponseObject(resp, err)
		},
	}
}

func createResponseObject(resp httpclient.HttpResponse, err error) map[string]interface{} {
	return map[string]interface{}{
		"response": resp,
		"error":    err,
		"assertStatus": func(expectedStatus int) map[string]interface{} {
			if resp.StatusCode != expectedStatus {
				panic(fmt.Sprintf("Expected status %d but got %d", expectedStatus, resp.StatusCode))
			}
			return map[string]interface{}{
				"response": resp,
				"error":    err,
			}
		},
	}
}

// createGroupModule handles the grouping of operations and sends group metrics.
func createGroupModule(metricsChan chan<- metrics.Metrics) map[string]interface{} {
	return map[string]interface{}{
		"start": func(name string, fn goja.Callable) {
			start := time.Now()
			fn(nil, nil) // Execute the group function
			duration := time.Since(start)
			metricsData := metrics.CollectGroupMetrics(name, duration)
			if metricsChan != nil {
				metrics.SendMetrics(metricsData, metricsChan)
			}
		},
	}
}

// createAssertModule provides basic assertion functionalities.
func createAssertModule() map[string]interface{} {
	return map[string]interface{}{
		"equal": func(expected, actual interface{}) {
			if expected != actual {
				panic(fmt.Sprintf("Assertion failed: expected %v, got %v", expected, actual))
			}
		},
		"notEqual": func(expected, actual interface{}) {
			if expected == actual {
				panic(fmt.Sprintf("Assertion failed: expected something different from %v, got %v", expected, actual))
			}
		},
	}
}

// createFSModule provides basic file system operations.
func SetupFSModule(vm *goja.Runtime) {
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

// createCryptoModule provides cryptographic operations such as random bytes generation, hashing, and HMAC.
func SetupCryptoModule(vm *goja.Runtime) {
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
				return util.Base64Encode(hash.Sum(nil))
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
				return util.Base64Encode(h.Sum(nil))
			},
		})
	})

	vm.Set("crypto", crypto)
}

// createFSModule provides basic file system operations.
// func createFSModule() map[string]interface{} {
// 	return map[string]interface{}{
// 		"readFileSync": func(filename string, encoding string) string {
// 			data, err := os.ReadFile(filename)
// 			if err != nil {
// 				return fmt.Sprintf("Error: %v", err)
// 			}
// 			return string(data)
// 		},
// 	}
// }

// createCryptoModule provides cryptographic operations such as random bytes generation, hashing, and HMAC.
// func createCryptoModule() map[string]interface{} {
// 	return map[string]interface{}{
// 		"randomBytes": func(size int) []byte {
// 			bytes := make([]byte, size)
// 			_, err := rand.Read(bytes)
// 			if err != nil {
// 				return nil
// 			}
// 			return bytes
// 		},
// 		"createHash": func(algorithm string) map[string]interface{} {
// 			hash := sha256.New()
// 			return map[string]interface{}{
// 				"update": func(data string) {
// 					hash.Write([]byte(data))
// 				},
// 				"digest": func(encoding string) string {
// 					return util.Base64Encode(hash.Sum(nil))
// 				},
// 			}
// 		},
// 		"createHmac": func(algorithm string, key string) map[string]interface{} {
// 			h := hmac.New(sha256.New, []byte(key))
// 			return map[string]interface{}{
// 				"update": func(data string) {
// 					h.Write([]byte(data))
// 				},
// 				"digest": func(encoding string) string {
// 					return util.Base64Encode(h.Sum(nil))
// 				},
// 			}
// 		},
// 	}
// }

func SetupJsonWebTokenModule(vm *goja.Runtime) {
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
func SetupConsoleModule(vm *goja.Runtime) {
	console := vm.NewObject()

	console.Set("log", func(call goja.FunctionCall) goja.Value {
		args := make([]interface{}, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		fmt.Println(args...)
		return nil
	})

	console.Set("error", func(call goja.FunctionCall) goja.Value {
		args := make([]interface{}, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		fmt.Fprintln(os.Stderr, args...)
		return nil
	})

	vm.Set("console", console)
}

func InitializeModuleExport(vm *goja.Runtime) *goja.Object {
	module := vm.NewObject()
	exports := vm.NewObject()
	module.Set("exports", exports)

	vm.Set("module", module)
	vm.Set("exports", exports)
	return module
}