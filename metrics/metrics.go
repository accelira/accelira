// File: metrics/metrics.go
package metrics

import (
	"fmt"
	"sync/atomic"
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

type EndpointMetricsAtomic struct {
	// Atomic counters
	TotalRequests      int64 // atomic
	TotalBytesReceived int64 // atomic
	TotalBytesSent     int64 // atomic
	TotalErrors        int64 // atomic
	TotalCheckPassed   int64 // atomic
	TotalCheckFailed   int64 // atomic

	// Status code counts: HTTP status codes 0–599
	StatusCodeCounts [600]int64 // atomic increments

	// Total response time in nanoseconds (for average calculation)
	TotalResponseTime int64 // atomic

	// Per-worker (goroutine) local slices for response times and handshake latencies
	// These are not included here; see below for per-worker pattern.

	Type MetricType
}

// AddRequest atomically increments the request count and status code count.
func (m *EndpointMetricsAtomic) AddRequest(statusCode int, responseTime time.Duration, bytesReceived, bytesSent int, isError, checkPassed, checkFailed bool) {
	atomic.AddInt64(&m.TotalRequests, 1)
	if statusCode >= 0 && statusCode < 600 {
		atomic.AddInt64(&m.StatusCodeCounts[statusCode], 1)
	}
	atomic.AddInt64(&m.TotalBytesReceived, int64(bytesReceived))
	atomic.AddInt64(&m.TotalBytesSent, int64(bytesSent))
	atomic.AddInt64(&m.TotalResponseTime, responseTime.Nanoseconds())
	if isError {
		atomic.AddInt64(&m.TotalErrors, 1)
	}
	if checkPassed {
		atomic.AddInt64(&m.TotalCheckPassed, 1)
	}
	if checkFailed {
		atomic.AddInt64(&m.TotalCheckFailed, 1)
	}
}

// For latency percentiles, each worker should collect response times in a local slice:
// var localResponseTimes []float64
// At aggregation time, merge all slices into a global t-digest.

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
