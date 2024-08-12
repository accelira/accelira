package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cheynewallace/tabby"
	"github.com/dop251/goja"
	"github.com/fatih/color"
	"github.com/influxdata/tdigest"
)

type EndpointMetrics struct {
	URL                string
	Method             string
	Requests           int
	TotalDuration      time.Duration
	TotalResponseTime  time.Duration
	TotalBytesReceived int
	TotalBytesSent     int
	StatusCodeCounts   map[int]int
	Errors             int
	ResponseTimes      *tdigest.TDigest
}

type Metrics struct {
	EndpointMetricsMap map[string]*EndpointMetrics
}

type HttpResponse struct {
	Body       string
	StatusCode int
}

func httpRequest(url string, method string, body io.Reader, metricsChan chan<- Metrics) (HttpResponse, error) {
	start := time.Now()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		select {
		case metricsChan <- Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{}}:
		default:
			fmt.Println("Channel is full, dropping metrics")
		}
		return HttpResponse{}, err
	}

	req.Header.Set("User-Agent", "Accelira perf testing tool/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		select {
		case metricsChan <- Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{}}:
		default:
			fmt.Println("Channel is full, dropping metrics")
		}
		return HttpResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		select {
		case metricsChan <- Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{}}:
		default:
			fmt.Println("Channel is full, dropping metrics")
		}
		return HttpResponse{}, err
	}

	duration := time.Since(start)
	metrics := Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{}}
	key := fmt.Sprintf("%s %s", method, url)
	if _, exists := metrics.EndpointMetricsMap[key]; !exists {
		metrics.EndpointMetricsMap[key] = &EndpointMetrics{
			URL:              url,
			Method:           method,
			StatusCodeCounts: make(map[int]int),
			ResponseTimes:    tdigest.New(),
		}
	}

	epMetrics := metrics.EndpointMetricsMap[key]
	epMetrics.Requests++
	epMetrics.TotalDuration += duration
	epMetrics.TotalResponseTime += duration
	epMetrics.TotalBytesReceived += len(responseBody)
	epMetrics.TotalBytesSent += len(req.URL.String())
	epMetrics.StatusCodeCounts[resp.StatusCode]++
	epMetrics.ResponseTimes.Add(float64(duration.Milliseconds()), 1)

	select {
	case metricsChan <- metrics:
	default:
		fmt.Println("Channel is full, dropping metrics")
	}

	return HttpResponse{
		Body:       string(responseBody),
		StatusCode: resp.StatusCode,
	}, nil
}

type Config struct {
	iterations      int
	rampUpRate      int // Users per second
	concurrentUsers int
}

func createConfigModule(config *Config) map[string]interface{} {
	return map[string]interface{}{
		"setIterations": func(iterations int) {
			config.iterations = iterations
		},
		"setRampUpRate": func(rate int) {
			config.rampUpRate = rate
		},
		"setConcurrentUsers": func(users int) {
			config.concurrentUsers = users
		},
		"getIterations": func() int {
			return config.iterations
		},
		"getRampUpRate": func() int {
			return config.rampUpRate
		},
		"getConcurrentUsers": func() int {
			return config.concurrentUsers
		},
	}
}

func runScript(script string, metricsChan chan<- Metrics, wg *sync.WaitGroup, config *Config) {
	defer wg.Done()

	vm := goja.New()
	configModule := createConfigModule(config)

	vm.Set("console", map[string]interface{}{
		"log": func(args ...interface{}) {
			for _, arg := range args {
				fmt.Println(arg)
			}
		},
	})

	modules := map[string]map[string]interface{}{
		"http": {
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
		},
		"assert": {
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
		},
		"config": configModule,
	}

	vm.Set("require", func(moduleName string) interface{} {
		if module, ok := modules[moduleName]; ok {
			return module
		}
		return nil
	})

	iterations := modules["config"]["getIterations"].(func() int)()

	fmt.Printf("  the iterations value is     : %v\n", iterations)
	for i := 0; i < iterations; i++ {
		wrappedScript := fmt.Sprintf("(function() { %s })();", script)

		_, err := vm.RunScript("script.js", wrappedScript)
		if err != nil {
			fmt.Println("Error running script:", err)
		}
	}
}

func generateReport(metricsList []Metrics) {
	aggregatedMetrics := make(map[string]*EndpointMetrics)
	for _, metrics := range metricsList {
		for key, epMetrics := range metrics.EndpointMetricsMap {
			if _, exists := aggregatedMetrics[key]; !exists {
				aggregatedMetrics[key] = &EndpointMetrics{
					StatusCodeCounts: make(map[int]int),
					ResponseTimes:    tdigest.New(),
				}
			}
			aggMetrics := aggregatedMetrics[key]
			aggMetrics.Requests += epMetrics.Requests
			aggMetrics.TotalDuration += epMetrics.TotalDuration
			aggMetrics.TotalResponseTime += epMetrics.TotalResponseTime
			aggMetrics.TotalBytesReceived += epMetrics.TotalBytesReceived
			aggMetrics.TotalBytesSent += epMetrics.TotalBytesSent
			aggMetrics.Errors += epMetrics.Errors

			aggMetrics.ResponseTimes.Add(epMetrics.ResponseTimes.Quantile(0.5), 1)
			for i := 0; i < int(epMetrics.ResponseTimes.Count()); i++ {
				aggMetrics.ResponseTimes.Add(epMetrics.ResponseTimes.Quantile(0.5), 1)
			}

			for code, count := range epMetrics.StatusCodeCounts {
				aggMetrics.StatusCodeCounts[code] += count
			}
		}
	}

	color.Cyan("=== Performance Test Report ===")
	fmt.Println()

	// Summary statistics
	totalRequests := 0
	totalErrors := 0
	totalDuration := time.Duration(0)
	for _, epMetrics := range aggregatedMetrics {
		totalRequests += epMetrics.Requests
		totalErrors += epMetrics.Errors
		totalDuration += epMetrics.TotalDuration
	}

	color.Green("Summary:")
	fmt.Printf("  Total Requests       : %d\n", totalRequests)
	fmt.Printf("  Total Errors         : %d\n", totalErrors)
	fmt.Printf("  Total Duration       : %v\n", totalDuration)

	if totalRequests > 0 {
		fmt.Printf("  Average Duration     : %v\n", totalDuration/time.Duration(totalRequests))
	} else {
		fmt.Println("  Average Duration     : N/A (No requests made)")
	}
	fmt.Println()

	// Create a table for endpoint metrics
	t := tabby.New()
	t.AddHeader("Endpoint", "Total Requests", "Total Duration", "Average Response Time", "50th Percentile", "90th Percentile", "Total Bytes Received", "Total Bytes Sent", "Errors")

	for key, epMetrics := range aggregatedMetrics {
		var avgResponseTime interface{}
		if epMetrics.Requests > 0 {
			avgResponseTime = epMetrics.TotalResponseTime / time.Duration(epMetrics.Requests)
		} else {
			avgResponseTime = "N/A"
		}

		t.AddLine(
			key,
			epMetrics.Requests,
			epMetrics.TotalDuration,
			avgResponseTime,
			epMetrics.ResponseTimes.Quantile(0.50),
			epMetrics.ResponseTimes.Quantile(0.90),
			epMetrics.TotalBytesReceived,
			epMetrics.TotalBytesSent,
			epMetrics.Errors,
		)
	}

	t.Print()

	color.Cyan("=== End of Report ===")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <script.js>")
		return
	}

	scriptPath := os.Args[1]

	script, err := os.ReadFile(scriptPath)
	if err != nil {
		fmt.Println("Error reading script file:", err)
		return
	}

	metricsChan := make(chan Metrics, 10000)

	var wg sync.WaitGroup
	var metricsList []Metrics
	var metricsMutex sync.Mutex

	metricsWaitGroup := sync.WaitGroup{}
	metricsWaitGroup.Add(1)

	go func() {
		for metrics := range metricsChan {
			metricsMutex.Lock()
			metricsList = append(metricsList, metrics)
			metricsMutex.Unlock()
		}
		metricsWaitGroup.Done()
	}()

	config := &Config{
		iterations:      10,
		rampUpRate:      1, // Default to 1 user per second
		concurrentUsers: 5,
	}

	vm := goja.New()
	configModule := createConfigModule(config)

	vm.Set("require", func(moduleName string) interface{} {
		if moduleName == "config" {
			return configModule
		}
		return nil
	})

	_, err = vm.RunScript("script.js", string(script))
	if err != nil {
		fmt.Println("Error running script:", err)
		return
	}

	concurrentUsers := configModule["getConcurrentUsers"].(func() int)()
	rampUpRate := configModule["getRampUpRate"].(func() int)()

	var sleepDuration time.Duration
	if rampUpRate > 0 {
		sleepDuration = time.Second / time.Duration(rampUpRate)
	} else {
		sleepDuration = 0 // If rampUpRate is 0, no sleep (immediate start)
	}

	for i := 0; i < concurrentUsers; i++ {
		wg.Add(1)
		go runScript(string(script), metricsChan, &wg, config)

		if sleepDuration > 0 && i < concurrentUsers-1 {
			time.Sleep(sleepDuration)
		}
		fmt.Printf("  Current loaded users       : %d\n", i)
	}

	wg.Wait()
	close(metricsChan)

	metricsWaitGroup.Wait()

	metricsMutex.Lock()
	generateReport(metricsList)
	metricsMutex.Unlock()
}
