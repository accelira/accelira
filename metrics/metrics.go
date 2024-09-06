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
		ResponseTime:     0,
		Type:             Group,
	}

	epMetrics.ResponseTime = duration

	return Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{key: epMetrics}}
}

func CollectErrorMetrics(name string, result bool) Metrics {
	key := name
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

// type EndpointMetrics struct {
// 	Type                       MetricType
// 	URL                        string
// 	Method                     string
// 	StatusCodeCounts           map[int]int
// 	ResponseTimes              time.Duration
// 	ResponseTimesTDigest       *tdigest.TDigest
// 	Requests                   int
// 	TotalResponseTime          time.Duration
// 	TotalBytesReceived         int
// 	TotalBytesSent             int
// 	Errors                     int
// 	TCPHandshakeLatency        time.Duration
// 	TCPHandshakeLatencyTDigest *tdigest.TDigest
// 	DNSLookupLatency           time.Duration
// 	DNSLookupLatencyTDigest    *tdigest.TDigest
// 	TLSHandshakeLatency        time.Duration
// 	TLSHandshakeLatencyTDigest *tdigest.TDigest
// 	BodySendLatency            time.Duration
// 	BodyReceiveLatency         time.Duration
// 	CheckResult                bool
// 	TotalCheckPassed           int
// 	TotalCheckFailed           int
// }

type EndpointMetrics struct {
	Type                MetricType
	URL                 string
	Method              string
	ResponseTime        time.Duration
	TCPHandshakeLatency time.Duration
	DNSLookupLatency    time.Duration
	TLSHandshakeLatency time.Duration
	BodySendLatency     time.Duration
	BodyReceiveLatency  time.Duration
	CheckResult         bool
	StatusCodeCounts    map[int]int
	BytesReceived       int
	BytesSent           int
	Errors              int
}

type EndpointMetricsAggregated struct {
	StatusCodeCounts           map[int]int
	TotalRequests              int
	TotalResponseTime          time.Duration
	ResponseTimesTDigest       *tdigest.TDigest
	TotalBytesReceived         int
	TotalBytesSent             int
	TotalErrors                int
	TCPHandshakeLatencyTDigest *tdigest.TDigest
	DNSLookupLatencyTDigest    *tdigest.TDigest
	TLSHandshakeLatencyTDigest *tdigest.TDigest
	TotalCheckPassed           int
	TotalCheckFailed           int
	Type                       MetricType
}
