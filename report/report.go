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
	metricsMap *map[string]*metrics.EndpointMetrics
}

// NewReportGenerator creates a new ReportGenerator instance.
func NewReportGenerator(metricsMap *map[string]*metrics.EndpointMetrics) *ReportGenerator {
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

	totalRequests, totalErrors, totalDuration := rg.aggregateMetrics()

	fmt.Printf("  Total Requests:   %d\n", totalRequests)
	fmt.Printf("  Total Errors:     %d\n", totalErrors)
	fmt.Printf("  Total Duration:   %v\n", totalDuration)
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
func (rg *ReportGenerator) printCheckStatus(key string, epMetrics *metrics.EndpointMetrics) {
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
func (rg *ReportGenerator) getCheckStatus(epMetrics *metrics.EndpointMetrics) (string, color.Attribute) {
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
func (rg *ReportGenerator) aggregateMetrics() (totalRequests, totalErrors int, totalDuration time.Duration) {

	for _, epMetrics := range *rg.metricsMap {
		if epMetrics.Type == metrics.HTTPRequest {
			totalRequests += epMetrics.Requests
			totalErrors += epMetrics.Errors
			totalDuration += epMetrics.TotalResponseTime
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
func (rg *ReportGenerator) printEndpointMetrics(endpoint string, epMetrics *metrics.EndpointMetrics) {
	avg := rg.roundDurationToTwoDecimals(epMetrics.TotalResponseTime / time.Duration(epMetrics.Requests))
	min := rg.quantileDuration(epMetrics, 0.0)
	med := rg.quantileDuration(epMetrics, 0.5)
	max := rg.quantileDuration(epMetrics, 1.0)
	p90 := rg.quantileDuration(epMetrics, 0.9)
	p95 := rg.quantileDuration(epMetrics, 0.95)

	dots := rg.generateDots(endpoint, 35) // Adjust total length as needed

	fmt.Printf("  %s%s avg=%v min=%v med=%v max=%v p(90)=%v p(95)=%v\n",
		endpoint, dots, avg, min, med, max, p90, p95)
}

// quantileDuration calculates the duration for a specific quantile from the TDigest.
func (rg *ReportGenerator) quantileDuration(epMetrics *metrics.EndpointMetrics, quantile float64) time.Duration {
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
