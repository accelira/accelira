package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/accelira/accelira/report"
	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/golang-jwt/jwt/v4"
	"github.com/influxdata/tdigest"
)

type HttpResponse struct {
	Body       string
	StatusCode int
}

func httpRequest(url, method string, body io.Reader, metricsChan chan<- report.Metrics) (HttpResponse, error) {
	start := time.Now()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return HttpResponse{}, err
	}

	req.Header.Set("User-Agent", "Accelira perf testing tool/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HttpResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return HttpResponse{}, err
	}

	duration := time.Since(start)
	metrics := collectMetrics(url, method, len(responseBody), len(req.URL.String()), resp.StatusCode, duration)
	sendMetrics(metrics, metricsChan)

	// if metricsChan != nil {
	// 	// Print the log in a sleek, color-coded format on the same line
	// 	fmt.Printf(
	// 		"\r\033[1;36m[\033[0m\033[1;34m%s\033[0m\033[1;36m]\033[0m \033[1;32m%s\033[0m - \033[1;31mStatus:\033[0m \033[1;31m%d\033[0m, \033[1;33mDuration:\033[0m \033[1;35m%v\033[0m",
	// 		method, url, resp.StatusCode, duration,
	// 	)

	// 	// fmt.Print("\r\n") // Add a new line after the log for clarity
	// }

	return HttpResponse{Body: string(responseBody), StatusCode: resp.StatusCode}, nil
}

func collectMetrics(url, method string, bytesReceived, bytesSent, statusCode int, duration time.Duration) report.Metrics {
	key := fmt.Sprintf("%s %s", method, url)
	epMetrics := &report.EndpointMetrics{
		URL:              url,
		Method:           method,
		StatusCodeCounts: make(map[int]int),
		ResponseTimes:    tdigest.New(),
	}

	epMetrics.Requests++
	epMetrics.TotalDuration += duration
	epMetrics.TotalResponseTime += duration
	epMetrics.TotalBytesReceived += bytesReceived
	epMetrics.TotalBytesSent += bytesSent
	epMetrics.StatusCodeCounts[statusCode]++
	epMetrics.ResponseTimes.Add(float64(duration.Milliseconds()), 1)

	return report.Metrics{EndpointMetricsMap: map[string]*report.EndpointMetrics{key: epMetrics}}
}

func sendMetrics(metrics report.Metrics, metricsChan chan<- report.Metrics) {
	if metricsChan != nil {
		select {
		case metricsChan <- metrics:
		default:
			fmt.Println("Channel is full, dropping metrics")
		}
	}
}

type Config struct {
	iterations      int
	rampUpRate      int
	concurrentUsers int
}

func createConfigModule(config *Config) map[string]interface{} {
	return map[string]interface{}{
		"setIterations":      func(iterations int) { config.iterations = iterations },
		"setRampUpRate":      func(rate int) { config.rampUpRate = rate },
		"setConcurrentUsers": func(users int) { config.concurrentUsers = users },
		"getIterations":      func() int { return config.iterations },
		"getRampUpRate":      func() int { return config.rampUpRate },
		"getConcurrentUsers": func() int { return config.concurrentUsers },
	}
}

func createGroupModule(metricsChan chan<- report.Metrics) map[string]interface{} {
	return map[string]interface{}{
		"start": func(name string, fn goja.Callable) {
			start := time.Now()
			fn(nil, nil) // Execute the group function
			duration := time.Since(start)
			metrics := collectGroupMetrics(name, duration)
			if metricsChan != nil {
				sendMetrics(metrics, metricsChan)
			}
		},
	}
}

func collectGroupMetrics(name string, duration time.Duration) report.Metrics {
	key := fmt.Sprintf("group: %s", name)
	epMetrics := &report.EndpointMetrics{
		URL:              name,
		Method:           "GROUP",
		StatusCodeCounts: make(map[int]int),
		ResponseTimes:    tdigest.New(),
	}

	epMetrics.Requests++
	epMetrics.TotalDuration += duration
	epMetrics.TotalResponseTime += duration
	epMetrics.ResponseTimes.Add(float64(duration.Milliseconds()), 1)

	return report.Metrics{EndpointMetricsMap: map[string]*report.EndpointMetrics{key: epMetrics}}
}

func runScript(script string, metricsChan chan<- report.Metrics, wg *sync.WaitGroup, config *Config) {
	defer wg.Done()

	vm := goja.New()
	setupFsModule(vm)
	setupCryptoModule(vm)
	setupJsonWebTokenModule(vm)
	setupConsoleModule(vm)
	vm.Set("require", setupRequire(config, metricsChan))

	iterations := config.iterations
	for i := 0; i < iterations; i++ {
		_, err := vm.RunScript("script.js", fmt.Sprintf("(function() { %s })();", script))
		if err != nil {
			fmt.Println("Error running script:", err)
		}
	}
}

func createConfigVM(content string) (*goja.Runtime, *Config, error) {
	vm := goja.New()
	config := &Config{}
	setupFsModule(vm)
	setupCryptoModule(vm)
	setupJsonWebTokenModule(vm)
	setupConsoleModule(vm)

	vm.Set("require", setupRequire(config, nil)) // Pass the correct arguments

	_, err := vm.RunScript("config.js", string(content))
	if err != nil {
		return nil, nil, fmt.Errorf("error running configuration script: %w", err)
	}

	return vm, config, nil
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

func setupRequire(config *Config, metricsChan chan<- report.Metrics) func(moduleName string) interface{} {
	return func(moduleName string) interface{} {
		switch moduleName {
		case "Accelira/http":
			return map[string]interface{}{
				"get": func(url string) (map[string]interface{}, error) {
					resp, err := httpRequest(url, "GET", nil, metricsChan)
					return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
				},
				"post": func(url string, body string) (map[string]interface{}, error) {
					resp, err := httpRequest(url, "POST", strings.NewReader(body), metricsChan)
					return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
				},
				"put": func(url string, body string) (map[string]interface{}, error) {
					resp, err := httpRequest(url, "PUT", strings.NewReader(body), metricsChan)
					return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
				},
				"delete": func(url string) (map[string]interface{}, error) {
					resp, err := httpRequest(url, "DELETE", nil, metricsChan)
					return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
				},
			}
		case "Accelira/assert":
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
		case "Accelira/config":
			return createConfigModule(config)
		case "Accelira/group":
			return createGroupModule(metricsChan)
		case "fs":
			return map[string]interface{}{
				"readFileSync": func(filename string, encoding string) string {
					data, err := os.ReadFile(filename)
					if err != nil {
						return fmt.Sprintf("Error: %v", err)
					}
					return string(data)
				},
			}
		case "crypto":
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
							return base64Encode(hash.Sum(nil))
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
							return base64Encode(h.Sum(nil))
						},
					}
				},
			}
		case "jsonwebtoken":
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
		default:
			return nil
		}
	}
}

func repeat(char rune, n int) string {
	result := make([]rune, n)
	for i := range result {
		result[i] = char
	}
	return string(result)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide the JavaScript file path as an argument")
		return
	}
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

	filePath := os.Args[1]
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{filePath},
		Bundle:      true,
		// Write:       false,
		Format:   api.FormatCommonJS,
		Platform: api.PlatformNeutral,
		Target:   api.ES2015,
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

	_, config, err := createConfigVM(string(code))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Concurrent Users: %d\n", config.concurrentUsers)
	fmt.Printf("Iterations: %d\n", config.iterations)
	fmt.Printf("Ramp-up Rate: %d\n", config.rampUpRate)

	metricsChan := make(chan report.Metrics, 10000)
	metricsList := make([]report.Metrics, 0)
	var mu sync.Mutex
	metricsWG := &sync.WaitGroup{}

	// Goroutine to process metrics
	go func() {
		defer metricsWG.Done() // Mark the metrics processing as done when the goroutine exits
		for metrics := range metricsChan {
			mu.Lock()
			metricsList = append(metricsList, metrics)
			mu.Unlock()
		}
	}()
	metricsWG.Add(1)
	// Run the scripts
	wg := &sync.WaitGroup{}
	progressWidth := 40
	for i := 0; i < config.concurrentUsers; i++ {
		percent := (i + 1) * 100 / config.concurrentUsers
		filled := percent * progressWidth / 100
		bar := fmt.Sprintf("[%s%s] %d%% (Current: %d / Target: %d)",
			repeat('=', filled),
			repeat(' ', progressWidth-filled),
			percent,
			i+1,
			config.concurrentUsers,
		)

		fmt.Printf("\r%s", bar)
		os.Stdout.Sync() // Ensure the progress bar is flushed to the output
		wg.Add(1)
		go runScript(string(code), metricsChan, wg, config)
		time.Sleep(time.Duration(1000/config.rampUpRate) * time.Millisecond)

	}

	wg.Wait()

	// fmt.Printf("\r\033[K")
	close(metricsChan) // Safe to close the channel now

	metricsWG.Wait() // Wait for the metrics processing to complete

	report.GenerateReport(metricsList)
}
