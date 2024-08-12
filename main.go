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

func httpRequest(url, method string, body io.Reader, metricsChan chan<- Metrics) (HttpResponse, error) {
	start := time.Now()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		sendEmptyMetrics(metricsChan)
		return HttpResponse{}, err
	}

	req.Header.Set("User-Agent", "Accelira perf testing tool/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		sendEmptyMetrics(metricsChan)
		return HttpResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		sendEmptyMetrics(metricsChan)
		return HttpResponse{}, err
	}

	duration := time.Since(start)
	metrics := collectMetrics(url, method, len(responseBody), len(req.URL.String()), resp.StatusCode, duration)
	sendMetrics(metrics, metricsChan)

	return HttpResponse{Body: string(responseBody), StatusCode: resp.StatusCode}, nil
}

func sendEmptyMetrics(metricsChan chan<- Metrics) {
	select {
	case metricsChan <- Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{}}:
	default:
		fmt.Println("Channel is full, dropping metrics")
	}
}

func collectMetrics(url, method string, bytesReceived, bytesSent, statusCode int, duration time.Duration) Metrics {
	key := fmt.Sprintf("%s %s", method, url)
	epMetrics := &EndpointMetrics{
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

	return Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{key: epMetrics}}
}

func sendMetrics(metrics Metrics, metricsChan chan<- Metrics) {
	select {
	case metricsChan <- metrics:
	default:
		fmt.Println("Channel is full, dropping metrics")
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

func createGroupModule(metricsChan chan<- Metrics, groupWG *sync.WaitGroup) map[string]interface{} {
	return map[string]interface{}{
		"start": func(name string, fn goja.Callable) {
			groupWG.Add(1)
			start := time.Now()
			fn(nil, nil) // Execute the group function
			duration := time.Since(start)
			metrics := collectGroupMetrics(name, duration)
			if metricsChan != nil {
				sendMetrics(metrics, metricsChan)
			}
			groupWG.Done()
		},
	}
}

func collectGroupMetrics(name string, duration time.Duration) Metrics {
	key := fmt.Sprintf("group: %s", name)
	epMetrics := &EndpointMetrics{
		URL:              name,
		Method:           "GROUP",
		StatusCodeCounts: make(map[int]int),
		ResponseTimes:    tdigest.New(),
	}

	epMetrics.Requests++
	epMetrics.TotalDuration += duration
	epMetrics.TotalResponseTime += duration
	epMetrics.ResponseTimes.Add(float64(duration.Milliseconds()), 1)

	return Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{key: epMetrics}}
}

func runScript(script string, metricsChan chan<- Metrics, wg *sync.WaitGroup, config *Config, groupWG *sync.WaitGroup) {
	defer wg.Done()

	vm := goja.New()
	vm.Set("console", createConsoleModule())
	vm.Set("require", createRequireModule(config, metricsChan, vm, groupWG))

	iterations := config.iterations
	for i := 0; i < iterations; i++ {
		_, err := vm.RunScript("script.js", fmt.Sprintf("(function() { %s })();", script))
		if err != nil {
			fmt.Println("Error running script:", err)
		}
	}
}

func createConsoleModule() map[string]interface{} {
	return map[string]interface{}{
		"log": func(args ...interface{}) {
			for _, arg := range args {
				fmt.Println(arg)
			}
		},
	}
}

func createRequireModule(config *Config, metricsChan chan<- Metrics, vm *goja.Runtime, groupWG *sync.WaitGroup) func(moduleName string) interface{} {
	return func(moduleName string) interface{} {
		switch moduleName {
		case "http":
			return map[string]interface{}{
				"get": func(url string) (map[string]interface{}, error) {
					if metricsChan == nil {
						return nil, nil
					}
					resp, err := httpRequest(url, "GET", nil, metricsChan)
					return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
				},
				"post": func(url string, body string) (map[string]interface{}, error) {
					if metricsChan == nil {
						return nil, nil
					}
					resp, err := httpRequest(url, "POST", strings.NewReader(body), metricsChan)
					return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
				},
				"put": func(url string, body string) (map[string]interface{}, error) {
					if metricsChan == nil {
						return nil, nil
					}
					resp, err := httpRequest(url, "PUT", strings.NewReader(body), metricsChan)
					return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
				},
				"delete": func(url string) (map[string]interface{}, error) {
					if metricsChan == nil {
						return nil, nil
					}
					resp, err := httpRequest(url, "DELETE", nil, metricsChan)
					return map[string]interface{}{"body": resp.Body, "status": resp.StatusCode}, err
				},
			}
		case "assert":
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
		case "config":
			return createConfigModule(config)
		case "group":
			return createGroupModule(metricsChan, groupWG)
		default:
			return nil
		}
	}
}

func generateReport(metricsList []Metrics) {
	aggregatedMetrics := aggregateMetrics(metricsList)
	printSummary(aggregatedMetrics)
	printDetailedReport(aggregatedMetrics)
}

func aggregateMetrics(metricsList []Metrics) map[string]*EndpointMetrics {
	aggregatedMetrics := make(map[string]*EndpointMetrics)
	for _, metrics := range metricsList {
		for key, epMetrics := range metrics.EndpointMetricsMap {
			if _, exists := aggregatedMetrics[key]; !exists {
				aggregatedMetrics[key] = &EndpointMetrics{
					StatusCodeCounts: make(map[int]int),
					ResponseTimes:    tdigest.New(),
				}
			}
			mergeEndpointMetrics(aggregatedMetrics[key], epMetrics)
		}
	}
	return aggregatedMetrics
}

func mergeEndpointMetrics(dest, src *EndpointMetrics) {
	dest.Requests += src.Requests
	dest.TotalDuration += src.TotalDuration
	dest.TotalResponseTime += src.TotalResponseTime
	dest.TotalBytesReceived += src.TotalBytesReceived
	dest.TotalBytesSent += src.TotalBytesSent
	dest.Errors += src.Errors
	dest.ResponseTimes.Add(src.ResponseTimes.Quantile(0.5), 1)
	for code, count := range src.StatusCodeCounts {
		dest.StatusCodeCounts[code] += count
	}
}

func printSummary(aggregatedMetrics map[string]*EndpointMetrics) {
	color.New(color.FgCyan).Add(color.Bold).Println("=== Performance Test Report ===")
	color.New(color.FgGreen).Add(color.Bold).Println("Summary:")
	totalRequests, totalErrors, totalDuration := 0, 0, time.Duration(0)
	for _, epMetrics := range aggregatedMetrics {
		totalRequests += epMetrics.Requests
		totalErrors += epMetrics.Errors
		totalDuration += epMetrics.TotalDuration
	}
	fmt.Printf("  Total Requests       : %d\n", totalRequests)
	fmt.Printf("  Total Errors         : %d\n", totalErrors)
	fmt.Printf("  Total Duration       : %v\n", totalDuration)
	fmt.Printf("  Average Duration     : %v\n", totalDuration/time.Duration(totalRequests))
	fmt.Println()
}

func printDetailedReport(aggregatedMetrics map[string]*EndpointMetrics) {
	color.New(color.FgGreen).Add(color.Bold).Println("Detailed Report:")
	t := tabby.New()
	t.AddHeader("Endpoint", "Requests", "Errors", "Avg. Response Time", "Status Codes")
	for key, epMetrics := range aggregatedMetrics {
		statusCodes := make([]string, 0)
		for code, count := range epMetrics.StatusCodeCounts {
			statusCodes = append(statusCodes, fmt.Sprintf("%d: %d", code, count))
		}
		t.AddLine(key, epMetrics.Requests, epMetrics.Errors, epMetrics.TotalResponseTime/time.Duration(epMetrics.Requests), strings.Join(statusCodes, ", "))
	}
	t.Print()
}

func createConfigVM(content string) (*goja.Runtime, *Config, error) {
	vm := goja.New()
	config := &Config{}
	metricsChan := make(chan Metrics, 10000)
	groupWG := &sync.WaitGroup{}

	vm.Set("console", createConsoleModule())
	vm.Set("require", createRequireModule(config, metricsChan, vm, groupWG)) // Pass the correct arguments

	_, err := vm.RunScript("config.js", string(content))
	if err != nil {
		return nil, nil, fmt.Errorf("error running configuration script: %w", err)
	}

	return vm, config, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide the JavaScript file path as an argument")
		return
	}

	filePath := os.Args[1]
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	_, config, err := createConfigVM(string(content))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Concurrent Users: %d\n", config.concurrentUsers)
	fmt.Printf("Iterations: %d\n", config.iterations)
	fmt.Printf("Ramp-up Rate: %d\n", config.rampUpRate)

	metricsChan := make(chan Metrics, 10000)
	metricsList := make([]Metrics, 0)
	groupWG := &sync.WaitGroup{}

	go func() {
		for metrics := range metricsChan {
			metricsList = append(metricsList, metrics)
		}
	}()

	wg := &sync.WaitGroup{}
	for i := 0; i < config.concurrentUsers; i++ {
		wg.Add(1)
		go runScript(string(content), metricsChan, wg, config, groupWG)
		time.Sleep(time.Duration(1000/config.rampUpRate) * time.Millisecond)
	}

	wg.Wait()
	groupWG.Wait() // Ensure all groups have finished
	close(metricsChan)

	generateReport(metricsList)
}
