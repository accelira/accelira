package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/accelira/accelira/metrics"
	"github.com/influxdata/tdigest"
)

type HttpResponse struct {
	Body       string
	StatusCode int
	URL        string
	Method     string
	Duration   time.Duration
}

func HttpRequest(url, method string, body io.Reader, metricsChannel chan<- metrics.Metrics) (HttpResponse, error) {
	start := time.Now()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return HttpResponse{}, err
	}

	req.Header.Set("User-Agent", "Accelira perf testing tool/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HttpResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return HttpResponse{}, err
	}

	duration := time.Since(start)
	metrics := collectMetrics(url, method, len(responseBody), len(req.URL.String()), resp.StatusCode, duration)
	sendMetrics(metrics, metricsChannel)

	return HttpResponse{
		Body:       string(responseBody),
		StatusCode: resp.StatusCode,
		URL:        url,
		Method:     method,
		Duration:   duration,
	}, nil
}

func collectMetrics(url, method string, bytesReceived, bytesSent, statusCode int, duration time.Duration) metrics.Metrics {
	key := fmt.Sprintf("%s %s", method, url)
	epMetrics := &metrics.EndpointMetrics{
		URL:              url,
		Method:           method,
		StatusCodeCounts: make(map[int]int),
		ResponseTimes:    tdigest.New(),
	}

	epMetrics.Requests++
	epMetrics.TotalDuration += duration
	epMetrics.TotalResponseTime += duration
	epMetrics.TotalBytesReceived += bytesReceived
	epMetrics.TotalBytesSent += bytesSent
	epMetrics.StatusCodeCounts[statusCode]++
	epMetrics.ResponseTimes.Add(float64(duration.Milliseconds()), 1)

	return metrics.Metrics{EndpointMetricsMap: map[string]*metrics.EndpointMetrics{key: epMetrics}}
}

func sendMetrics(metrics metrics.Metrics, metricsChan chan<- metrics.Metrics) {
	if metricsChan != nil {
		select {
		case metricsChan <- metrics:
		default:
			fmt.Println("Channel is full, dropping metrics")
		}
	}
}
