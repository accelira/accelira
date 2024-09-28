package report

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/accelira/accelira/metrics"
	"github.com/fatih/color"
)

// ReportGenerator handles the generation of performance reports.
type ReportGenerator struct {
	metricsMap *map[string]*metrics.EndpointMetricsAggregated
}

// NewReportGenerator creates a new ReportGenerator instance.
func NewReportGenerator(metricsMap *map[string]*metrics.EndpointMetricsAggregated) *ReportGenerator {
	return &ReportGenerator{
		metricsMap: metricsMap,
	}
}

// GenerateReport generates a detailed report for the performance test.
func (rg *ReportGenerator) GenerateReport() {
	rg.printSummary()
	rg.printChecks()
	rg.printDetailedReport()
}

// printSummary prints the summary of the performance test.
func (rg *ReportGenerator) printSummary() {
	color.New(color.FgCyan, color.Bold).Println("\nPerformance Test Report")
	color.New(color.FgWhite).Println("\nSummary:")

	totalRequests, totalErrors, totalDuration, totalBytesReceived, totalBytesSent := rg.aggregateMetrics()

	fmt.Printf("  Total Requests:   %d\n", totalRequests)
	fmt.Printf("  Total Errors:     %d\n", totalErrors)
	fmt.Printf("  Total Duration:   %v\n", totalDuration)
	fmt.Printf("  Total BytesReceived:   %v\n", totalBytesReceived)
	fmt.Printf("  Total BytesSent:   %v\n", totalBytesSent)

	rg.printAverageDuration(totalRequests, totalDuration)
}

// printChecks prints the status of various checks.
func (rg *ReportGenerator) printChecks() {
	color.New(color.FgMagenta).Println("\nChecks Status:")

	for key, epMetrics := range *rg.metricsMap {
		if epMetrics.Type == metrics.Error {
			rg.printCheckStatus(key, epMetrics)
		}
	}
}

// printCheckStatus prints the status of an individual check.
func (rg *ReportGenerator) printCheckStatus(key string, epMetrics *metrics.EndpointMetricsAggregated) {
	checkStatus, statusColor := rg.getCheckStatus(epMetrics)

	statusLine := fmt.Sprintf("  %s %s", checkStatus, key)
	color.New(statusColor).Println(statusLine)

	totalChecks := epMetrics.TotalCheckPassed + epMetrics.TotalCheckFailed
	passRate := rg.calculateRate(epMetrics.TotalCheckPassed, totalChecks)
	failRate := rg.calculateRate(epMetrics.TotalCheckFailed, totalChecks)

	fmt.Printf("    Pass Rate: %.2f%% (%d / %d) | Fail Rate: %.2f%% (%d / %d)\n",
		passRate, epMetrics.TotalCheckPassed, totalChecks,
		failRate, epMetrics.TotalCheckFailed, totalChecks)
}

// getCheckStatus determines the status and color of the check.
func (rg *ReportGenerator) getCheckStatus(epMetrics *metrics.EndpointMetricsAggregated) (string, color.Attribute) {
	if epMetrics.TotalCheckFailed > 0 {
		return "✗ Failed", color.FgRed
	}
	return "✓ Passed", color.FgGreen
}

// calculateRate calculates the percentage rate given a count and total.
func (rg *ReportGenerator) calculateRate(count, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) / float64(total) * 100
}

// aggregateMetrics aggregates the total requests, errors, and duration from all endpoints.
func (rg *ReportGenerator) aggregateMetrics() (totalRequests, totalErrors int, totalDuration time.Duration, totalBytesReceived int, totalBytesSent int) {

	for _, epMetrics := range *rg.metricsMap {
		if epMetrics.Type == metrics.HTTPRequest {
			totalRequests += epMetrics.TotalRequests
			totalErrors += epMetrics.TotalErrors
			totalDuration += epMetrics.TotalResponseTime
			totalBytesReceived += epMetrics.TotalBytesReceived
			totalBytesSent += epMetrics.TotalBytesSent
		}
	}
	return
}

// printAverageDuration prints the average duration of the requests if available.
func (rg *ReportGenerator) printAverageDuration(totalRequests int, totalDuration time.Duration) {
	if totalRequests > 0 {
		avgDuration := totalDuration / time.Duration(totalRequests)
		fmt.Printf("  Average Duration: %v\n", avgDuration)
	} else {
		fmt.Println("  Average Duration: N/A")
	}
}

// printDetailedReport prints detailed metrics for each endpoint.
func (rg *ReportGenerator) printDetailedReport() {
	color.New(color.FgWhite, color.Bold).Println("\nEndpoint Metrics:")

	for endpoint, epMetrics := range *rg.metricsMap {
		if epMetrics.Type == metrics.HTTPRequest || epMetrics.Type == metrics.Group {
			rg.printEndpointMetrics(endpoint, epMetrics)
		}
	}
}

// printEndpointMetrics prints the metrics for a specific endpoint.
func (rg *ReportGenerator) printEndpointMetrics(endpoint string, epMetrics *metrics.EndpointMetricsAggregated) {
	avg := rg.roundDurationToTwoDecimals(epMetrics.TotalResponseTime / time.Duration(epMetrics.TotalRequests))
	min := rg.quantileDuration(epMetrics, 0.0)
	med := rg.quantileDuration(epMetrics, 0.5)
	max := rg.quantileDuration(epMetrics, 1.0)
	p90 := rg.quantileDuration(epMetrics, 0.9)
	p95 := rg.quantileDuration(epMetrics, 0.95)

	// TCP Handshake Latency
	tcpMin := rg.quantileTCPHandshakeDuration(epMetrics, 0.0)
	tcpMed := rg.quantileTCPHandshakeDuration(epMetrics, 0.5)
	tcpMax := rg.quantileTCPHandshakeDuration(epMetrics, 1.0)
	tcpP90 := rg.quantileTCPHandshakeDuration(epMetrics, 0.9)
	tcpP95 := rg.quantileTCPHandshakeDuration(epMetrics, 0.95)

	// DNS Lookup Latency
	dnsMin := rg.quantileDNSLookupDuration(epMetrics, 0.0)
	dnsMed := rg.quantileDNSLookupDuration(epMetrics, 0.5)
	dnsMax := rg.quantileDNSLookupDuration(epMetrics, 1.0)
	dnsP90 := rg.quantileDNSLookupDuration(epMetrics, 0.9)
	dnsP95 := rg.quantileDNSLookupDuration(epMetrics, 0.95)

	// TLS Handshake Latency
	tlsMin := rg.quantileTLSHandshakeDuration(epMetrics, 0.0)
	tlsMed := rg.quantileTLSHandshakeDuration(epMetrics, 0.5)
	tlsMax := rg.quantileTLSHandshakeDuration(epMetrics, 1.0)
	tlsP90 := rg.quantileTLSHandshakeDuration(epMetrics, 0.9)
	tlsP95 := rg.quantileTLSHandshakeDuration(epMetrics, 0.95)

	dots := rg.generateDots(endpoint, 35) // Adjust total length as needed

	fmt.Printf("  %s%s avg=%v min=%v med=%v max=%v p(90)=%v p(95)=%v\n",
		endpoint, dots, avg, min, med, max, p90, p95)

	if epMetrics.Type == metrics.HTTPRequest {
		if epMetrics.TCPHandshakeLatencyTDigest != nil {
			fmt.Printf("    └── TCP Handshake Latency: min=%v med=%v max=%v p(90)=%v p(95)=%v\n", tcpMin, tcpMed, tcpMax, tcpP90, tcpP95)
		}

		if epMetrics.DNSLookupLatencyTDigest != nil {
			fmt.Printf("    └── DNS Lookup Latency: min=%v med=%v max=%v p(90)=%v p(95)=%v\n", dnsMin, dnsMed, dnsMax, dnsP90, dnsP95)
		}

		if epMetrics.TLSHandshakeLatencyTDigest != nil {
			fmt.Printf("    └── TLS Handshake Latency: min=%v med=%v max=%v p(90)=%v p(95)=%v\n", tlsMin, tlsMed, tlsMax, tlsP90, tlsP95)
		}
	}
}

func (rg *ReportGenerator) quantileTLSHandshakeDuration(epMetrics *metrics.EndpointMetricsAggregated, quantile float64) time.Duration {
	if epMetrics.TLSHandshakeLatencyTDigest != nil {
		return time.Duration(epMetrics.TLSHandshakeLatencyTDigest.Quantile(quantile)) * time.Millisecond
	}
	return 0
}

func (rg *ReportGenerator) quantileDNSLookupDuration(epMetrics *metrics.EndpointMetricsAggregated, quantile float64) time.Duration {
	if epMetrics.DNSLookupLatencyTDigest != nil {
		return time.Duration(epMetrics.DNSLookupLatencyTDigest.Quantile(quantile)) * time.Millisecond
	}
	return 0
}

// quantileTCPHandshakeDuration calculates the TCP handshake latency for a specific quantile.
func (rg *ReportGenerator) quantileTCPHandshakeDuration(epMetrics *metrics.EndpointMetricsAggregated, quantile float64) time.Duration {
	if epMetrics.TCPHandshakeLatencyTDigest != nil {
		return time.Duration(epMetrics.TCPHandshakeLatencyTDigest.Quantile(quantile)) * time.Millisecond
	}
	return 0
}

// quantileDuration calculates the duration for a specific quantile from the TDigest.
func (rg *ReportGenerator) quantileDuration(epMetrics *metrics.EndpointMetricsAggregated, quantile float64) time.Duration {
	return time.Duration(epMetrics.ResponseTimesTDigest.Quantile(quantile)) * time.Millisecond
}

// generateDots generates the dots for alignment in the report.
func (rg *ReportGenerator) generateDots(endpoint string, totalLength int) string {
	numDots := totalLength - len(endpoint)
	if numDots < 0 {
		numDots = 0
	}
	return strings.Repeat(".", numDots)
}

// roundDurationToTwoDecimals rounds the duration to two decimal places.
func (rg *ReportGenerator) roundDurationToTwoDecimals(d time.Duration) time.Duration {
	seconds := d.Seconds()
	roundedSeconds := math.Round(seconds*100) / 100
	return time.Duration(roundedSeconds * float64(time.Second))
}
