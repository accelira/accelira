package metricsprocessor

import (
	"sync"
	"sync/atomic"

	"github.com/accelira/accelira/metrics"
	"github.com/influxdata/tdigest"
)

const numShards = 16

// WorkerMetrics is a per-worker (goroutine) struct for zero-contention metrics collection.
type WorkerMetrics struct {
	EndpointKey      string
	TotalRequests    int
	TotalBytesRecv   int
	TotalBytesSent   int
	TotalErrors      int
	TotalCheckPassed int
	TotalCheckFailed int
	StatusCodeCounts [600]int
	ResponseTimes    []float64 // for percentiles
	// Add more fields as needed (e.g., handshake latencies)
}

// Reset clears the worker metrics for reuse.
func (wm *WorkerMetrics) Reset() {
	wm.TotalRequests = 0
	wm.TotalBytesRecv = 0
	wm.TotalBytesSent = 0
	wm.TotalErrors = 0
	wm.TotalCheckPassed = 0
	wm.TotalCheckFailed = 0
	for i := range wm.StatusCodeCounts {
		wm.StatusCodeCounts[i] = 0
	}
	wm.ResponseTimes = wm.ResponseTimes[:0]
}

// ShardedMetricsAggregator holds global metrics, sharded for low lock contention.
type ShardedMetricsAggregator struct {
	Shards [numShards]struct {
		Mutex  sync.Mutex
		Global map[string]*metrics.EndpointMetricsAtomic // key: endpoint
	}
}

// NewShardedMetricsAggregator initializes the aggregator.
func NewShardedMetricsAggregator() *ShardedMetricsAggregator {
	agg := &ShardedMetricsAggregator{}
	for i := 0; i < numShards; i++ {
		agg.Shards[i].Global = make(map[string]*metrics.EndpointMetricsAtomic)
	}
	return agg
}

// shardFor returns the shard index for a given endpoint key.
func shardFor(key string) int {
	h := 0
	for i := 0; i < len(key); i++ {
		h = 31*h + int(key[i])
	}
	return h % numShards
}

// Aggregate merges a worker's metrics into the global aggregator.
func (agg *ShardedMetricsAggregator) Aggregate(wm *WorkerMetrics) {
	idx := shardFor(wm.EndpointKey)
	shard := &agg.Shards[idx]
	shard.Mutex.Lock()
	defer shard.Mutex.Unlock()

	m, ok := shard.Global[wm.EndpointKey]
	if !ok {
		m = &metrics.EndpointMetricsAtomic{Type: metrics.HTTPRequest} // Use HTTP_REQUEST as default
		shard.Global[wm.EndpointKey] = m
	}
	// Atomically aggregate counters
	m.TotalRequests += int64(wm.TotalRequests)
	m.TotalBytesReceived += int64(wm.TotalBytesRecv)
	m.TotalBytesSent += int64(wm.TotalBytesSent)
	m.TotalErrors += int64(wm.TotalErrors)
	m.TotalCheckPassed += int64(wm.TotalCheckPassed)
	m.TotalCheckFailed += int64(wm.TotalCheckFailed)
	for i := 0; i < 600; i++ {
		m.StatusCodeCounts[i] += int64(wm.StatusCodeCounts[i])
	}
	m.TotalResponseTime += int64(sumFloat64s(wm.ResponseTimes))
	// Note: For percentiles, collect all response times from all workers and merge into t-digest at report time.
}

// sumFloat64s returns the sum of a slice of float64s.
func sumFloat64s(a []float64) float64 {
	sum := 0.0
	for _, v := range a {
		sum += v
	}
	return sum
}

// --- Example usage of the new optimized metrics flow ---

// ExampleWorker simulates a worker collecting metrics and flushing to the aggregator.
func ExampleWorker(agg *ShardedMetricsAggregator, endpoint string, results []float64) {
	wm := &WorkerMetrics{EndpointKey: endpoint}
	for _, respTime := range results {
		wm.TotalRequests++
		wm.StatusCodeCounts[200]++ // Example: all 200 OK
		wm.ResponseTimes = append(wm.ResponseTimes, respTime)
	}
	agg.Aggregate(wm)
}

// ExampleReport generates a summary from the aggregator and computes percentiles using t-digest.
func ExampleReport(agg *ShardedMetricsAggregator) {
	// allRespTimes := make([]float64, 0, 1024)
	for i := 0; i < numShards; i++ {
		shard := &agg.Shards[i]
		shard.Mutex.Lock()
		for endpoint, m := range shard.Global {
			println("Endpoint:", endpoint)
			println("  TotalRequests:", m.TotalRequests)
			println("  Status 200:", m.StatusCodeCounts[200])
			// Collect for percentiles
			// In real usage, you would merge per-worker response times here
			// For demo, just print average
			if m.TotalRequests > 0 {
				avg := float64(m.TotalResponseTime) / float64(m.TotalRequests) / 1e6 // ms
				println("  Avg Response Time (ms):", avg)
			}
		}
		shard.Mutex.Unlock()
	}
	// To compute percentiles:
	// 1. Collect all response times from all workers (or store globally)
	// 2. Merge into t-digest
	// td := tdigest.New()
	// for _, t := range allRespTimes { td.Add(t, 1) }
	// println("P95:", td.Quantile(0.95))
}

// --- Legacy logic below ---
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
