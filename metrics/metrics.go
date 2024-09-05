// File: metrics/metrics.go
package metrics

import (
	"fmt"
	"time"

	"github.com/influxdata/tdigest"
)

func SendMetrics(metrics Metrics, metricsChan chan<- Metrics) {
	select {
	case metricsChan <- metrics:
	default:
		fmt.Println("Channel is full, dropping metrics")
	}
}

func NewTDigest() *tdigest.TDigest {
	return tdigest.New()
}

func CollectGroupMetrics(name string, duration time.Duration) Metrics {
	key := fmt.Sprintf("group: %s", name)
	epMetrics := &EndpointMetrics{
		URL:              name,
		Method:           "GROUP",
		StatusCodeCounts: make(map[int]int),
		ResponseTimes:    0,
		Type:             Group,
	}

	epMetrics.Requests = 1
	epMetrics.TotalResponseTime += duration
	epMetrics.ResponseTimes = duration

	return Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{key: epMetrics}}
}

func CollectErrorMetrics(name string, result bool) Metrics {
	key := fmt.Sprintf("%s", name)
	epMetrics := &EndpointMetrics{
		URL:         name,
		Method:      "ERROR",
		Type:        Error,
		CheckResult: result,
	}

	return Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{key: epMetrics}}
}

type Metrics struct {
	EndpointMetricsMap map[string]*EndpointMetrics
}

type MetricType string

const (
	HTTPRequest MetricType = "HTTP_REQUEST"
	Error       MetricType = "ERROR"
	Group       MetricType = "GROUP"
)

type EndpointMetrics struct {
	Type                       MetricType
	URL                        string
	Method                     string
	StatusCodeCounts           map[int]int
	ResponseTimes              time.Duration
	ResponseTimesTDigest       *tdigest.TDigest
	Requests                   int
	TotalResponseTime          time.Duration
	TotalBytesReceived         int
	TotalBytesSent             int
	Errors                     int
	TCPHandshakeLatency        time.Duration
	TCPHandshakeLatencyTDigest *tdigest.TDigest
	DNSLookupLatency           time.Duration
	DNSLookupLatencyTDigest    *tdigest.TDigest
	CheckResult                bool
	TotalCheckPassed           int
	TotalCheckFailed           int
}
