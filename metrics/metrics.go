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

func CollectGroupMetrics(name string, duration time.Duration) Metrics {
	key := fmt.Sprintf("group: %s", name)
	epMetrics := &EndpointMetrics{
		URL:              name,
		Method:           "GROUP",
		StatusCodeCounts: make(map[int]int),
		ResponseTimes:    tdigest.New(),
	}

	epMetrics.Requests++
	epMetrics.TotalDuration += duration
	epMetrics.TotalResponseTime += duration
	epMetrics.ResponseTimes.Add(float64(duration.Milliseconds()), 1)

	return Metrics{EndpointMetricsMap: map[string]*EndpointMetrics{key: epMetrics}}
}

type Metrics struct {
	EndpointMetricsMap map[string]*EndpointMetrics
}

type EndpointMetrics struct {
	URL                string
	Method             string
	Requests           int
	TotalDuration      time.Duration
	TotalResponseTime  time.Duration
	TotalBytesReceived int
	TotalBytesSent     int
	Errors             int
	StatusCodeCounts   map[int]int
	ResponseTimes      *tdigest.TDigest
}

func AggregateMetrics(metricsList []Metrics) map[string]*EndpointMetrics {
	aggregatedMetrics := make(map[string]*EndpointMetrics)
	for _, metrics := range metricsList {
		for key, epMetrics := range metrics.EndpointMetricsMap {
			if _, exists := aggregatedMetrics[key]; !exists {
				aggregatedMetrics[key] = &EndpointMetrics{
					StatusCodeCounts: make(map[int]int),
					ResponseTimes:    tdigest.New(),
				}
			}
			mergeEndpointMetrics(aggregatedMetrics[key], epMetrics)
		}
	}
	return aggregatedMetrics
}

func mergeEndpointMetrics(dest, src *EndpointMetrics) {
	dest.Requests += src.Requests
	dest.TotalDuration += src.TotalDuration
	dest.TotalResponseTime += src.TotalResponseTime
	dest.TotalBytesReceived += src.TotalBytesReceived
	dest.TotalBytesSent += src.TotalBytesSent
	dest.Errors += src.Errors
	dest.ResponseTimes.Add(src.ResponseTimes.Quantile(0.5), 1)
	for code, count := range src.StatusCodeCounts {
		dest.StatusCodeCounts[code] += count
	}
}
