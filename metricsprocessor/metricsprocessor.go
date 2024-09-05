package metricsprocessor

import (
	"sync"
	"sync/atomic"

	"github.com/accelira/accelira/metrics"
	"github.com/influxdata/tdigest"
)

var (
	MetricsMap      = make(map[string]*metrics.EndpointMetrics)
	MetricsMapMutex sync.RWMutex
	MetricsReceived int32
)

func GatherMetrics(metricsChannel <-chan metrics.Metrics, metricsWaitGroup *sync.WaitGroup) {
	defer metricsWaitGroup.Done()

	for metric := range metricsChannel {
		processMetrics(metric)
	}
}

func processMetrics(metric metrics.Metrics) {
	for key, endpointMetric := range metric.EndpointMetricsMap {
		processEndpointMetric(key, endpointMetric)
	}
}

func processEndpointMetric(key string, endpointMetric *metrics.EndpointMetrics) {
	// MetricsMapMutex.RLock()
	storedMetric, isExisting := MetricsMap[key]
	// MetricsMapMutex.RUnlock()

	// fmt.Printf("storedMetric %v \n", storedMetric)

	if !isExisting {
		newMetric := initializeNewMetric(endpointMetric)
		// MetricsMapMutex.Lock()
		MetricsMap[key] = newMetric
		// MetricsMapMutex.Unlock()
		return
	}

	mergeMetrics(storedMetric, endpointMetric)
}

func initializeNewMetric(endpointMetric *metrics.EndpointMetrics) *metrics.EndpointMetrics {
	returnMetrics := &metrics.EndpointMetrics{
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

	returnMetrics.ResponseTimesTDigest.Add(float64(endpointMetric.ResponseTimes.Milliseconds()), 1)
	returnMetrics.TCPHandshakeLatencyTDigest.Add(float64(endpointMetric.TCPHandshakeLatency.Milliseconds()), 1)
	returnMetrics.DNSLookupLatencyTDigest.Add(float64(endpointMetric.DNSLookupLatency.Milliseconds()), 1)
	if endpointMetric.CheckResult {
		returnMetrics.TotalCheckPassed += 1
	} else {
		returnMetrics.TotalCheckFailed += 1
	}

	return returnMetrics
}

func mergeMetrics(storedMetric, newMetric *metrics.EndpointMetrics) {
	atomic.AddInt32(&MetricsReceived, 1)

	storedMetric.Requests += newMetric.Requests
	storedMetric.TotalResponseTime += newMetric.TotalResponseTime
	storedMetric.TotalBytesReceived += newMetric.TotalBytesReceived
	storedMetric.TotalBytesSent += newMetric.TotalBytesSent
	if newMetric.CheckResult {
		storedMetric.TotalCheckPassed += 1
	} else {
		storedMetric.TotalCheckFailed += 1
	}

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
