package metricsprocessor

import (
	"sync"
	"sync/atomic"

	"github.com/accelira/accelira/metrics"
	"github.com/influxdata/tdigest"
)

const numShards = 16

type MetricsShard struct {
	Map   map[string]*metrics.EndpointMetricsAggregated
	Mutex sync.RWMutex
}

var (
	metricsShards   [numShards]MetricsShard
	MetricsReceived int32
)

func init() {
	for i := 0; i < numShards; i++ {
		metricsShards[i].Map = make(map[string]*metrics.EndpointMetricsAggregated)
	}
}

func getShard(key string) *MetricsShard {
	h := fnv32(key)
	return &metricsShards[uint(h)%uint(numShards)]
}

func fnv32(key string) uint32 {
	var hash uint32 = 2166136261
	const prime32 = 16777619
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= prime32
	}
	return hash
}

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
	shard := getShard(key)
	shard.Mutex.Lock()
	defer shard.Mutex.Unlock()

	storedMetric, isExisting := shard.Map[key]
	if !isExisting {
		newMetric := initializeNewMetric(endpointMetric)
		shard.Map[key] = newMetric
		return
	}

	mergeMetrics(storedMetric, endpointMetric)
}

func initializeNewMetric(endpointMetric *metrics.EndpointMetrics) *metrics.EndpointMetricsAggregated {
	returnMetrics := &metrics.EndpointMetricsAggregated{
		ResponseTimesTDigest:       tdigest.New(),
		TCPHandshakeLatencyTDigest: tdigest.New(),
		DNSLookupLatencyTDigest:    tdigest.New(),
		TLSHandshakeLatencyTDigest: tdigest.New(),
		TotalRequests:              1,
		TotalResponseTime:          endpointMetric.ResponseTime,
		TotalBytesReceived:         endpointMetric.BytesReceived,
		TotalBytesSent:             endpointMetric.BytesSent,
		TotalErrors:                endpointMetric.Errors,
		StatusCodeCounts:           make(map[int]int),
		Type:                       endpointMetric.Type,
	}

	returnMetrics.ResponseTimesTDigest.Add(float64(endpointMetric.ResponseTime.Milliseconds()), 1)
	returnMetrics.TCPHandshakeLatencyTDigest.Add(float64(endpointMetric.TCPHandshakeLatency.Milliseconds()), 1)
	returnMetrics.DNSLookupLatencyTDigest.Add(float64(endpointMetric.DNSLookupLatency.Milliseconds()), 1)
	returnMetrics.TLSHandshakeLatencyTDigest.Add(float64(endpointMetric.TLSHandshakeLatency.Milliseconds()), 1)
	if endpointMetric.CheckResult {
		returnMetrics.TotalCheckPassed += 1
	} else {
		returnMetrics.TotalCheckFailed += 1
	}

	return returnMetrics
}

// GetAllAggregatedMetrics returns a merged map of all metrics from all shards.
func GetAllAggregatedMetrics() map[string]*metrics.EndpointMetricsAggregated {
	result := make(map[string]*metrics.EndpointMetricsAggregated)
	for i := 0; i < numShards; i++ {
		shard := &metricsShards[i]
		shard.Mutex.RLock()
		for k, v := range shard.Map {
			result[k] = v
		}
		shard.Mutex.RUnlock()
	}
	return result
}

func mergeMetrics(storedMetric *metrics.EndpointMetricsAggregated, newMetric *metrics.EndpointMetrics) {
	atomic.AddInt32(&MetricsReceived, 1)

	storedMetric.TotalRequests += 1
	storedMetric.TotalResponseTime += newMetric.ResponseTime
	storedMetric.TotalBytesReceived += newMetric.BytesReceived
	storedMetric.TotalBytesSent += newMetric.BytesSent
	storedMetric.TotalErrors += newMetric.Errors
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

func mergeTDigests(storedMetric *metrics.EndpointMetricsAggregated, newMetric *metrics.EndpointMetrics) {
	storedMetric.ResponseTimesTDigest.Add(float64(newMetric.ResponseTime.Milliseconds()), 1)
	if newMetric.TCPHandshakeLatency.Milliseconds() > 0 {
		storedMetric.TCPHandshakeLatencyTDigest.Add(float64(newMetric.TCPHandshakeLatency.Milliseconds()), 1)
	}
	if newMetric.DNSLookupLatency.Milliseconds() > 0 {
		storedMetric.DNSLookupLatencyTDigest.Add(float64(newMetric.DNSLookupLatency.Milliseconds()), 1)
	}
	if newMetric.TLSHandshakeLatency.Milliseconds() > 0 {
		storedMetric.TLSHandshakeLatencyTDigest.Add(float64(newMetric.TLSHandshakeLatency.Milliseconds()), 1)
	}
}
