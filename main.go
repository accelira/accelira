package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cheynewallace/tabby"
	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
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
		case "Accelira/http":
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

func roundDurationToTwoDecimals(d time.Duration) time.Duration {
	// Convert duration to seconds as a float64
	seconds := d.Seconds()

	// Round to two decimal places
	roundedSeconds := math.Round(seconds*100) / 100

	// Convert back to time.Duration
	return time.Duration(roundedSeconds * float64(time.Second))
}

func printDetailedReport(aggregatedMetrics map[string]*EndpointMetrics) {
	color.New(color.FgGreen).Add(color.Bold).Println("Detailed Report:")
	t := tabby.New()
	t.AddHeader("Endpoint", "Req.", "Errs", "Avg. Resp. Time", "50th % Latency", "95th % Latency", "Status Codes")
	for key, epMetrics := range aggregatedMetrics {
		statusCodes := make([]string, 0)
		for code, count := range epMetrics.StatusCodeCounts {
			statusCodes = append(statusCodes, fmt.Sprintf("%d: %d", code, count))
		}
		percentile50 := epMetrics.ResponseTimes.Quantile(0.5)
		percentile95 := epMetrics.ResponseTimes.Quantile(0.95)
		t.AddLine(key, epMetrics.Requests, epMetrics.Errors, roundDurationToTwoDecimals(epMetrics.TotalResponseTime/time.Duration(epMetrics.Requests)), time.Duration(percentile50)*time.Millisecond, time.Duration(percentile95)*time.Millisecond, strings.Join(statusCodes, ", "))
	}
	t.Print()
}

func createConfigVM(content string) (*goja.Runtime, *Config, error) {
	vm := goja.New()
	config := &Config{}
	groupWG := &sync.WaitGroup{}

	vm.Set("console", createConsoleModule())
	vm.Set("require", createRequireModule(config, nil, vm, groupWG)) // Pass the correct arguments

	_, err := vm.RunScript("config.js", string(content))
	if err != nil {
		return nil, nil, fmt.Errorf("error running configuration script: %w", err)
	}

	return vm, config, nil
}

func esbuildTransform(src, filename string) (string, error) {
	opts := api.TransformOptions{
		Sourcefile:     filename,
		Loader:         api.LoaderJS,
		Target:         api.ESNext,
		Format:         api.FormatIIFE, // Use IIFE to handle imports/exports
		Sourcemap:      api.SourceMapExternal,
		SourcesContent: api.SourcesContentInclude,
		LegalComments:  api.LegalCommentsNone,
		Platform:       api.PlatformNeutral,
		LogLevel:       api.LogLevelSilent,
		Charset:        api.CharsetUTF8,
	}

	result := api.Transform(src, opts)

	if len(result.Errors) > 0 {
		msg := result.Errors[0]
		err := fmt.Errorf("esbuild error: %s", msg.Text)
		if msg.Location != nil {
			err = fmt.Errorf("esbuild error: %s at %s:%d:%d", msg.Text, msg.Location.File, msg.Location.Line, msg.Location.Column)
		}
		return "", err
	}

	return string(result.Code), nil
}

// LoadModule loads a JavaScript module from a file and transforms it using esbuild
func LoadModule(runtime *goja.Runtime, modulePath string) (string, error) {
	content, err := ioutil.ReadFile(modulePath)
	if err != nil {
		return "", err
	}

	code, err := esbuildTransform(string(content), modulePath)
	if err != nil {
		return "", err
	}

	// Log the transformed code
	fmt.Printf("Transformed code for %s:\n%s\n", modulePath, code)

	return code, err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide the JavaScript file path as an argument")
		return
	}

	filePath := os.Args[1]
	_, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	runtime := goja.New()
	code, err := LoadModule(runtime, filePath)

	_, config, err := createConfigVM(string(code))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Concurrent Users: %d\n", config.concurrentUsers)
	fmt.Printf("Iterations: %d\n", config.iterations)
	fmt.Printf("Ramp-up Rate: %d\n", config.rampUpRate)

	metricsChan := make(chan Metrics, 10000)
	metricsList := make([]Metrics, 0)
	var mu sync.Mutex
	groupWG := &sync.WaitGroup{}
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
	metricsWG.Add(1) // Indicate that there is one goroutine processing metrics

	// Run the scripts
	wg := &sync.WaitGroup{}
	for i := 0; i < config.concurrentUsers; i++ {
		wg.Add(1)
		go runScript(string(code), metricsChan, wg, config, groupWG)
		time.Sleep(time.Duration(1000/config.rampUpRate) * time.Millisecond)
	}

	wg.Wait()
	groupWG.Wait()     // Ensure all groups have finished
	close(metricsChan) // Safe to close the channel now

	metricsWG.Wait() // Wait for the metrics processing to complete

	generateReport(metricsList)
}
