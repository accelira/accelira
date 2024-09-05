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
	Duration        time.Duration
}

func createConfigModule(config *Config) map[string]interface{} {
	return map[string]interface{}{
		"setIterations":      func(iterations int) { config.Iterations = iterations },
		"setRampUpRate":      func(rate int) { config.RampUpRate = rate },
		"setConcurrentUsers": func(users int) { config.ConcurrentUsers = users },
		"getIterations":      func() int { return config.Iterations },
		"getRampUpRate":      func() int { return config.RampUpRate },
		"getConcurrentUsers": func() int { return config.ConcurrentUsers },
		"setDuration": func(duration string) {
			parsedDuration, _ := time.ParseDuration(duration)
			config.Duration = parsedDuration
		},
		"getDuration": func() time.Duration { return config.Duration },
	}
}

func SetupRequire(vm *goja.Runtime, config *Config, metricsChan chan<- metrics.Metrics) func(moduleName string) interface{} {
	return func(moduleName string) interface{} {
		switch moduleName {
		case "Accelira/http":
			return createHTTPModule(metricsChan)
		case "Accelira/config":
			return createConfigModule(config)
		case "Accelira/group":
			return createGroupModule(metricsChan)
		case "Accelira/assert":
			return createAssertModule(metricsChan, vm) // Pass vm here
		case "fs":
			return createFSModule()
		case "crypto":
			return createCryptoModule()
		case "jsonwebtoken":
			return createJsonWebTokenModule()
		}
		return nil
	}
}

// createHTTPModule handles HTTP requests (GET, POST, PUT, DELETE) and sends metrics.
func createHTTPModule(metricsChan chan<- metrics.Metrics) map[string]interface{} {
	client := httpclient.NewHTTPClient()
	return map[string]interface{}{
		"get": func(url string) map[string]interface{} {
			resp, err := client.DoRequest(url, "GET", nil, metricsChan)
			return createResponseObject(resp, err, metricsChan)
		},
		"post": func(url string, body string) map[string]interface{} {
			resp, err := client.DoRequest(url, "POST", strings.NewReader(body), metricsChan)
			return createResponseObject(resp, err, metricsChan)
		},
		"put": func(url string, body string) map[string]interface{} {
			resp, err := client.DoRequest(url, "PUT", strings.NewReader(body), metricsChan)
			return createResponseObject(resp, err, metricsChan)
		},
		"delete": func(url string) map[string]interface{} {
			resp, err := client.DoRequest(url, "DELETE", nil, metricsChan)
			return createResponseObject(resp, err, metricsChan)
		},
	}
}

func createResponseObject(resp httpclient.HttpResponse, err error, metricsChan chan<- metrics.Metrics) map[string]interface{} {
	return map[string]interface{}{
		"response": resp,
		"error":    err,
		"assertStatus": func(expectedStatus int) map[string]interface{} {
			if resp.StatusCode != expectedStatus {
				// Send metrics for failed assertion

				if len(fmt.Sprintf("%s %s", resp.Method, resp.URL)) < 5 {
					fmt.Printf("blank error")
				}

				metricsData := metrics.Metrics{
					EndpointMetricsMap: map[string]*metrics.EndpointMetrics{
						fmt.Sprintf("%s %s", resp.Method, resp.URL): {
							URL:              resp.URL,
							Method:           resp.Method,
							StatusCodeCounts: map[int]int{resp.StatusCode: 1},
							Errors:           1,
							Type:             metrics.Error,
						},
					},
				}
				metrics.SendMetrics(metricsData, metricsChan)
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
func createAssertModule(metricsChan chan<- metrics.Metrics, vm *goja.Runtime) map[string]interface{} {
	return map[string]interface{}{
		"check": func(response map[string]interface{}, assertions map[string]interface{}) {
			for name, assertFunc := range assertions {
				// Check if the assertFunc is callable (goja.Callable)
				if fn, ok := assertFunc.(func(goja.FunctionCall) goja.Value); ok {
					responseValue := vm.ToValue(response["response"])

					funcCall := goja.FunctionCall{
						This:      goja.Undefined(),
						Arguments: []goja.Value{responseValue},
					}
					result := fn(funcCall)
					// if !result.ToBoolean() {

					metricsData := metrics.CollectErrorMetrics(name, result.ToBoolean())
					// if metricsChan != nil {
					metrics.SendMetrics(metricsData, metricsChan)
					// }
					// }
				} else {
					panic(fmt.Sprintf("Invalid assertion function for '%s'", name))
				}
			}
		},
	}
}

// createFSModule provides basic file system operations.
func createFSModule() map[string]interface{} {
	return map[string]interface{}{
		"readFileSync": func(filename string, encoding string) string {
			data, err := os.ReadFile(filename)
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}
			return string(data)
		},
	}
}

// createCryptoModule provides cryptographic operations such as random bytes generation, hashing, and HMAC.
func createCryptoModule() map[string]interface{} {
	return map[string]interface{}{
		"randomBytes": func(size int) []byte {
			bytes := make([]byte, size)
			_, err := rand.Read(bytes)
			if err != nil {
				return nil
			}
			return bytes
		},
		"createHash": func(algorithm string) map[string]interface{} {
			hash := sha256.New()
			return map[string]interface{}{
				"update": func(data string) {
					hash.Write([]byte(data))
				},
				"digest": func(encoding string) string {
					return util.Base64Encode(hash.Sum(nil))
				},
			}
		},
		"createHmac": func(algorithm string, key string) map[string]interface{} {
			h := hmac.New(sha256.New, []byte(key))
			return map[string]interface{}{
				"update": func(data string) {
					h.Write([]byte(data))
				},
				"digest": func(encoding string) string {
					return util.Base64Encode(h.Sum(nil))
				},
			}
		},
	}
}

func createJsonWebTokenModule() map[string]interface{} {
	return map[string]interface{}{
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
	}
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
