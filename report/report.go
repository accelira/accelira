// package report

// import (
// 	"fmt"
// 	"math"
// 	"strings"
// 	"sync"
// 	"time"

// 	"github.com/accelira/accelira/metrics"
// 	"github.com/cheynewallace/tabby"
// 	"github.com/fatih/color"
// )

// func GenerateReport1(metricsMap *sync.Map) {
// 	printSummary(metricsMap)
// 	color.New(color.FgGreen).Add(color.Bold).Println("\nDetailed Report:")
// 	t := tabby.New()
// 	t.AddHeader("Endpoint", "Req.", "Errs", "Avg. Resp. Time", "50th % Latency", "90th % Latency", "TCP Handshake Latency", "DNS Lookup Latency", "Status Codes")

// 	metricsMap.Range(func(key, value interface{}) bool {
// 		endpoint := key.(string)
// 		epMetrics := value.(*metrics.EndpointMetrics)

// 		statusCodes := make([]string, 0)
// 		for code, count := range epMetrics.StatusCodeCounts {
// 			statusCodes = append(statusCodes, fmt.Sprintf("%d: %d", code, count))
// 		}

// 		percentile50 := epMetrics.ResponseTimesTDigest.Quantile(0.5)
// 		percentile95 := epMetrics.ResponseTimesTDigest.Quantile(0.99)

// 		t.AddLine(
// 			endpoint,
// 			epMetrics.Requests,
// 			epMetrics.Errors,
// 			roundDurationToTwoDecimals(epMetrics.TotalResponseTime/time.Duration(epMetrics.Requests)),
// 			time.Duration(percentile50)*time.Millisecond,
// 			time.Duration(percentile95)*time.Millisecond,
// 			time.Duration(epMetrics.TCPHandshakeLatencyTDigest.Quantile(0.9))*time.Millisecond,
// 			time.Duration(epMetrics.DNSLookupLatencyTDigest.Quantile(0.9))*time.Millisecond,
// 			strings.Join(statusCodes, ", "),
// 		)
// 		return true
// 	})

// 	t.Print()
// }

// func printSummary(metricsMap *sync.Map) {
// 	color.New(color.FgCyan).Add(color.Bold).Println("\n=== Performance Test Report ===")
// 	color.New(color.FgGreen).Add(color.Bold).Println("\nSummary:")

// 	totalRequests, totalErrors, totalDuration := 0, 0, time.Duration(0)

// 	metricsMap.Range(func(key, value interface{}) bool {
// 		epMetrics := value.(*metrics.EndpointMetrics)
// 		totalRequests += epMetrics.Requests
// 		totalErrors += epMetrics.Errors
// 		totalDuration += epMetrics.TotalDuration
// 		return true
// 	})

// 	fmt.Printf("  Total Requests       : %d\n", totalRequests)
// 	fmt.Printf("  Total Errors         : %d\n", totalErrors)
// 	fmt.Printf("  Total Duration       : %v\n", totalDuration)
// 	if totalRequests > 0 {
// 		avgDuration := totalDuration / time.Duration(totalRequests)
// 		fmt.Printf("  Average Duration     : %v\n", avgDuration)
// 	} else {
// 		fmt.Println("  Average Duration     : N/A")
// 	}
// 	fmt.Println()
// }

// func roundDurationToTwoDecimals(d time.Duration) time.Duration {
// 	seconds := d.Seconds()
// 	roundedSeconds := math.Round(seconds*100) / 100
// 	return time.Duration(roundedSeconds * float64(time.Second))
// }

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

func GenerateReport1(metricsMap *sync.Map) {
	printSummary(metricsMap)
	color.New(color.FgGreen).Add(color.Bold).Println("\nDetailed Report:")

	metricsMap.Range(func(key, value interface{}) bool {
		endpoint := key.(string)
		epMetrics := value.(*metrics.EndpointMetrics)

		avg := roundDurationToTwoDecimals(epMetrics.TotalResponseTime / time.Duration(epMetrics.Requests))
		min := time.Duration(epMetrics.ResponseTimesTDigest.Quantile(0.0)) * time.Millisecond
		med := time.Duration(epMetrics.ResponseTimesTDigest.Quantile(0.5)) * time.Millisecond
		max := time.Duration(epMetrics.ResponseTimesTDigest.Quantile(1.0)) * time.Millisecond
		p90 := time.Duration(epMetrics.ResponseTimesTDigest.Quantile(0.9)) * time.Millisecond
		p95 := time.Duration(epMetrics.ResponseTimesTDigest.Quantile(0.95)) * time.Millisecond

		// Calculate the number of dots needed
		totalLength := 40 // Adjust this as needed for total length of endpoint + dots
		numDots := totalLength - len(endpoint)
		if numDots < 0 {
			numDots = 0
		}
		dots := strings.Repeat(".", numDots)

		// Print the formatted string
		fmt.Printf("  %s%s: avg=%v  min=%v  med=%v  max=%v  p(90)=%v  p(95)=%v\n",
			endpoint, dots, avg, min, med, max, p90, p95)

		return true
	})
}

func printSummary(metricsMap *sync.Map) {
	color.New(color.FgCyan).Add(color.Bold).Println("\n=== Performance Test Report ===")
	color.New(color.FgGreen).Add(color.Bold).Println("\nSummary:")

	totalRequests, totalErrors, totalDuration := 0, 0, time.Duration(0)

	metricsMap.Range(func(key, value interface{}) bool {
		epMetrics := value.(*metrics.EndpointMetrics)
		totalRequests += epMetrics.Requests
		totalErrors += epMetrics.Errors
		totalDuration += epMetrics.TotalDuration
		return true
	})

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
