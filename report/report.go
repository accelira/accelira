package report

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/accelira/accelira/metrics"
	"github.com/fatih/color"
)

// GenerateReport generates a detailed report for the performance test.
func GenerateReport(metricsMap *sync.Map) {
	printSummary(metricsMap)
	color.New(color.FgGreen).Add(color.Bold).Println("\nDetailed Report:")

	metricsMap.Range(func(key, value interface{}) bool {
		endpoint := key.(string)
		epMetrics := value.(*metrics.EndpointMetrics)

		printEndpointMetrics(endpoint, epMetrics)
		return true
	})
}

// printSummary prints the summary of the performance test.
func printSummary(metricsMap *sync.Map) {
	color.New(color.FgCyan).Add(color.Bold).Println("\n=== Performance Test Report ===")
	color.New(color.FgGreen).Add(color.Bold).Println("\nSummary:")

	totalRequests, totalErrors, totalDuration := aggregateMetrics(metricsMap)

	fmt.Printf("  Total Requests       : %d\n", totalRequests)
	fmt.Printf("  Total Errors         : %d\n", totalErrors)
	fmt.Printf("  Total Duration       : %v\n", totalDuration)
	printAverageDuration(totalRequests, totalDuration)
	fmt.Println()
}

// aggregateMetrics aggregates the total requests, errors, and duration from all endpoints.
func aggregateMetrics(metricsMap *sync.Map) (totalRequests, totalErrors int, totalDuration time.Duration) {
	metricsMap.Range(func(key, value interface{}) bool {
		epMetrics := value.(*metrics.EndpointMetrics)
		if epMetrics.Type == metrics.HTTPRequest {
			totalRequests += epMetrics.Requests
			totalErrors += epMetrics.Errors
			totalDuration += epMetrics.TotalDuration
		}
		return true
	})
	return
}

// printAverageDuration prints the average duration of the requests if available.
func printAverageDuration(totalRequests int, totalDuration time.Duration) {
	if totalRequests > 0 {
		avgDuration := totalDuration / time.Duration(totalRequests)
		fmt.Printf("  Average Duration     : %v\n", avgDuration)
	} else {
		fmt.Println("  Average Duration     : N/A")
	}
}

// printEndpointMetrics prints the metrics for a specific endpoint.
func printEndpointMetrics(endpoint string, epMetrics *metrics.EndpointMetrics) {
	avg := roundDurationToTwoDecimals(epMetrics.TotalResponseTime / time.Duration(epMetrics.Requests))
	min := quantileDuration(epMetrics, 0.0)
	med := quantileDuration(epMetrics, 0.5)
	max := quantileDuration(epMetrics, 1.0)
	p90 := quantileDuration(epMetrics, 0.9)
	p95 := quantileDuration(epMetrics, 0.95)

	dots := generateDots(endpoint, 40) // Adjust total length as needed

	fmt.Printf("  %s%s: avg=%v  min=%v  med=%v  max=%v  p(90)=%v  p(95)=%v\n",
		endpoint, dots, avg, min, med, max, p90, p95)
}

// quantileDuration calculates the duration for a specific quantile from the TDigest.
func quantileDuration(epMetrics *metrics.EndpointMetrics, quantile float64) time.Duration {
	return time.Duration(epMetrics.ResponseTimesTDigest.Quantile(quantile)) * time.Millisecond
}

// generateDots generates the dots for alignment in the report.
func generateDots(endpoint string, totalLength int) string {
	numDots := totalLength - len(endpoint)
	if numDots < 0 {
		numDots = 0
	}
	return strings.Repeat(".", numDots)
}

// roundDurationToTwoDecimals rounds the duration to two decimal places.
func roundDurationToTwoDecimals(d time.Duration) time.Duration {
	seconds := d.Seconds()
	roundedSeconds := math.Round(seconds*100) / 100
	return time.Duration(roundedSeconds * float64(time.Second))
}
