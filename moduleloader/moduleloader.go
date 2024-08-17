// File: moduleloader/moduleloader.go
package moduleloader

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/accelira/accelira/httpclient"
	"github.com/accelira/accelira/metrics" // Import the new metrics package
	"github.com/dop251/goja"
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
		case "fs":
			return createFSModule()
		case "crypto":
			return createCryptoModule()
		}
		return nil
	}
}

// createHTTPModule handles HTTP requests (GET, POST, PUT, DELETE) and sends metrics.
func createHTTPModule(metricsChan chan<- metrics.Metrics) map[string]interface{} {
	return map[string]interface{}{
		"get": func(url string) (map[string]interface{}, error) {
			resp, err := httpclient.HttpRequest(url, "GET", nil, metricsChan)
			return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
		},
		"post": func(url string, body string) (map[string]interface{}, error) {
			resp, err := httpclient.HttpRequest(url, "POST", strings.NewReader(body), metricsChan)
			return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
		},
		"put": func(url string, body string) (map[string]interface{}, error) {
			resp, err := httpclient.HttpRequest(url, "PUT", strings.NewReader(body), metricsChan)
			return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
		},
		"delete": func(url string) (map[string]interface{}, error) {
			resp, err := httpclient.HttpRequest(url, "DELETE", nil, metricsChan)
			return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
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
}

// base64Encode encodes data to a base64 string.
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
