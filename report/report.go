package report

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cheynewallace/tabby"
	"github.com/fatih/color"
	"github.com/influxdata/tdigest"
)

type Metrics struct {
	EndpointMetricsMap map[string]*EndpointMetrics
}

type EndpointMetrics struct {
	URL                string
	Method             string
	Requests           int
	TotalDuration      time.Duration
	TotalResponseTime  time.Duration
	TotalBytesReceived int
	TotalBytesSent     int
	Errors             int
	StatusCodeCounts   map[int]int
	ResponseTimes      *tdigest.TDigest
}

func GenerateReport(metricsList []Metrics) {
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
	color.New(color.FgCyan).Add(color.Bold).Println("\n=== Performance Test Report ===")
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
	if totalRequests > 0 {
		avgDuration := totalDuration / time.Duration(totalRequests)
		fmt.Printf("  Average Duration     : %v\n", avgDuration)
	} else {
		fmt.Println("  Average Duration     : N/A")
	}
	fmt.Println()
}

func roundDurationToTwoDecimals(d time.Duration) time.Duration {
	seconds := d.Seconds()
	roundedSeconds := math.Round(seconds*100) / 100
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
