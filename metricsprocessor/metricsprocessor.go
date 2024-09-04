package metricsprocessor

import (
	"sync"
	"sync/atomic"

	"github.com/accelira/accelira/metrics"
	"github.com/influxdata/tdigest"
)

var (
	MetricsMap sync.Map

	metricsReceived int32
)

func GatherMetrics(metricsChannel <-chan metrics.Metrics, metricsWaitGroup *sync.WaitGroup) {
	defer metricsWaitGroup.Done()

	for metric := range metricsChannel {
		processMetrics(metric)
	}
}

func processMetrics(metric metrics.Metrics) {
	for key, endpointMetric := range metric.EndpointMetricsMap {
		if endpointMetric.Type == metrics.HTTPRequest || endpointMetric.Type == metrics.Group {
			processEndpointMetric(key, endpointMetric)
		}
	}
}

func processEndpointMetric(key string, endpointMetric *metrics.EndpointMetrics) {
	value, isExisting := MetricsMap.LoadOrStore(key, initializeNewMetric(endpointMetric))
	storedMetric := value.(*metrics.EndpointMetrics)

	if isExisting {
		mergeMetrics(storedMetric, endpointMetric)
	}
}

func initializeNewMetric(endpointMetric *metrics.EndpointMetrics) *metrics.EndpointMetrics {
	return &metrics.EndpointMetrics{
		ResponseTimesTDigest:       tdigest.New(),
		TCPHandshakeLatencyTDigest: tdigest.New(),
		DNSLookupLatencyTDigest:    tdigest.New(),
		Requests:                   endpointMetric.Requests,
		TotalResponseTime:          endpointMetric.TotalResponseTime,
		TotalBytesReceived:         endpointMetric.TotalBytesReceived,
		TotalBytesSent:             endpointMetric.TotalBytesSent,
		StatusCodeCounts:           make(map[int]int),
		Type:                       endpointMetric.Type,
	}
}

func mergeMetrics(storedMetric, newMetric *metrics.EndpointMetrics) {
	atomic.AddInt32(&metricsReceived, 1)

	storedMetric.Requests += newMetric.Requests
	storedMetric.TotalResponseTime += newMetric.TotalResponseTime
	storedMetric.TotalBytesReceived += newMetric.TotalBytesReceived
	storedMetric.TotalBytesSent += newMetric.TotalBytesSent

	for statusCode, count := range newMetric.StatusCodeCounts {
		storedMetric.StatusCodeCounts[statusCode] += count
	}

	mergeTDigests(storedMetric, newMetric)
}

func mergeTDigests(storedMetric, newMetric *metrics.EndpointMetrics) {
	storedMetric.ResponseTimesTDigest.Add(float64(newMetric.ResponseTimes.Milliseconds()), 1)
	if newMetric.TCPHandshakeLatency.Milliseconds() > 0 {
		storedMetric.TCPHandshakeLatencyTDigest.Add(float64(newMetric.TCPHandshakeLatency.Milliseconds()), 1)
	}
	if newMetric.DNSLookupLatency.Milliseconds() > 0 {
		storedMetric.DNSLookupLatencyTDigest.Add(float64(newMetric.DNSLookupLatency.Milliseconds()), 1)
	}
}
