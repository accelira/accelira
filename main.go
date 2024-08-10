package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

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

func httpRequest(url string, method string, body io.Reader, metricsChan chan<- Metrics) (string, error) {
	start := time.Now()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		metricsChan <- Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{}}
		return "", err
	}

	req.Header.Set("User-Agent", "CustomTool/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		metricsChan <- Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{}}
		return "", err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		metricsChan <- Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{}}
		return "", err
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

	if err != nil {
		epMetrics.Errors++
	}

	metricsChan <- metrics

	return string(responseBody), nil
}

func runScript(script string, iterations int, metricsChan chan<- Metrics, wg *sync.WaitGroup) {
	defer wg.Done()

	vm := goja.New()

	// Define a basic console object
	vm.Set("console", map[string]interface{}{
		"log": func(args ...interface{}) {
			for _, arg := range args {
				fmt.Println(arg)
			}
		},
	})

	vm.Set("http", map[string]interface{}{
		"get": func(url string) (string, error) {
			response, err := httpRequest(url, "GET", nil, metricsChan)
			if err != nil {
				return fmt.Sprintf("Error: %s", err), nil
			}
			return response, nil
		},
		"post": func(url string, body string) (string, error) {
			response, err := httpRequest(url, "POST", strings.NewReader(body), metricsChan)
			if err != nil {
				return fmt.Sprintf("Error: %s", err), nil
			}
			return response, nil
		},
		"put": func(url string, body string) (string, error) {
			response, err := httpRequest(url, "PUT", strings.NewReader(body), metricsChan)
			if err != nil {
				return fmt.Sprintf("Error: %s", err), nil
			}
			return response, nil
		},
		"delete": func(url string) (string, error) {
			response, err := httpRequest(url, "DELETE", nil, metricsChan)
			if err != nil {
				return fmt.Sprintf("Error: %s", err), nil
			}
			return response, nil
		},
	})

	for i := 0; i < iterations; i++ {
		_, err := vm.RunScript("script.js", script)
		if err != nil {
			fmt.Println("Script Error:", err)
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

			// Merge response times
			aggMetrics.ResponseTimes.Add(epMetrics.ResponseTimes.Quantile(0.5), 1)
			for i := 0; i < int(epMetrics.ResponseTimes.Count()); i++ {
				aggMetrics.ResponseTimes.Add(epMetrics.ResponseTimes.Quantile(0.5), 1)
			}

			for code, count := range epMetrics.StatusCodeCounts {
				aggMetrics.StatusCodeCounts[code] += count
			}
		}
	}

	color.Cyan("=== Performance Test Report by Endpoint and Method ===")
	fmt.Println()

	for key, epMetrics := range aggregatedMetrics {
		color.Blue("Endpoint: %s", key)
		fmt.Printf("  Total Requests       : %d\n", epMetrics.Requests)
		fmt.Printf("  Total Duration       : %v\n", epMetrics.TotalDuration)
		fmt.Printf("  Average Response Time: %v\n", epMetrics.TotalResponseTime/time.Duration(epMetrics.Requests))
		fmt.Printf("  50th Percentile      : %v\n", epMetrics.ResponseTimes.Quantile(0.50))
		fmt.Printf("  90th Percentile      : %v\n", epMetrics.ResponseTimes.Quantile(0.90))
		fmt.Printf("  Total Bytes Received : %d\n", epMetrics.TotalBytesReceived)
		fmt.Printf("  Total Bytes Sent     : %d\n", epMetrics.TotalBytesSent)
		fmt.Printf("  Errors               : %d\n", epMetrics.Errors)
		fmt.Println()
		color.Green("  Status Code Counts:")
		for code, count := range epMetrics.StatusCodeCounts {
			fmt.Printf("    %d: %d\n", code, count)
		}
		fmt.Println()
	}

	color.Cyan("=== End of Report ===")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <script.js>")
		return
	}

	scriptPath := os.Args[1] // Get the script file path from the command line argument

	iterations := 10
	concurrentWorkers := 5

	// Read the script file once
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		fmt.Println("Error reading script file:", err)
		return
	}

	metricsChan := make(chan Metrics, iterations*concurrentWorkers)
	var wg sync.WaitGroup
	for i := 0; i < concurrentWorkers; i++ {
		wg.Add(1)
		go runScript(string(script), iterations, metricsChan, &wg)
	}
	go func() {
		wg.Wait()
		close(metricsChan)
	}()

	var metricsList []Metrics
	for metrics := range metricsChan {
		metricsList = append(metricsList, metrics)
	}

	generateReport(metricsList)
}
