package httpclient

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/accelira/accelira/metrics"
)

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient() *HTTPClient {
	transport := &http.Transport{
		DisableKeepAlives: false,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
	}

	return &HTTPClient{client: client}
}

func (hc *HTTPClient) DoRequest(url, method string, body io.Reader, metricsChannel chan<- metrics.Metrics) (HttpResponse, error) {
	var dnsStart, dnsEnd, connectStart, connectEnd, wroteRequestTime, GotFirstResponseByteTime time.Time

	trace := &httptrace.ClientTrace{
		DNSStart:     func(info httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:      func(info httptrace.DNSDoneInfo) { dnsEnd = time.Now() },
		ConnectStart: func(network, addr string) { connectStart = time.Now() },
		ConnectDone:  func(network, addr string, err error) { connectEnd = time.Now() },
		GotFirstResponseByte: func() {
			GotFirstResponseByteTime = time.Now()
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			wroteRequestTime = time.Now()
		},
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return HttpResponse{}, err
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	req.Header.Set("User-Agent", "Accelira perf testing tool/1.0")

	responseStartTime := time.Now()
	resp, err := hc.client.Do(req)
	if err != nil {
		// Check if the error is a timeout
		if e, ok := err.(net.Error); ok && e.Timeout() {
			return HttpResponse{
				Body:                "Request timed out",
				StatusCode:          http.StatusRequestTimeout,
				URL:                 url,
				Method:              method,
				Duration:            time.Since(responseStartTime),
				TCPHandshakeLatency: 0,
				DNSLookupLatency:    0,
			}, nil
		}

		// Check if the error is a connection refused error
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" && opErr.Err.Error() == "connection refused" {
			return HttpResponse{
				Body:                "Connection refused",
				StatusCode:          http.StatusServiceUnavailable,
				URL:                 url,
				Method:              method,
				Duration:            time.Since(responseStartTime),
				TCPHandshakeLatency: 0,
				DNSLookupLatency:    0,
			}, nil
		}

		// Handle other network errors
		if opErr, ok := err.(*net.OpError); ok {
			return HttpResponse{
				Body:                "Network error: " + opErr.Error(),
				StatusCode:          http.StatusNetworkAuthenticationRequired,
				URL:                 url,
				Method:              method,
				Duration:            time.Since(responseStartTime),
				TCPHandshakeLatency: 0,
				DNSLookupLatency:    0,
			}, nil
		}

		// Handle other types of errors
		return HttpResponse{
			Body:                "An unexpected error occurred",
			StatusCode:          http.StatusInternalServerError,
			URL:                 url,
			Method:              method,
			Duration:            time.Since(responseStartTime),
			TCPHandshakeLatency: 0,
			DNSLookupLatency:    0,
		}, err
	}

	responseEndTime := time.Now()
	totalResponseTime := responseEndTime.Sub(responseStartTime)

	defer resp.Body.Close()

	duration := GotFirstResponseByteTime.Sub(wroteRequestTime)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return HttpResponse{}, err
	}

	tcpHandshakeLatency := connectEnd.Sub(connectStart)
	dnsLookupLatency := dnsEnd.Sub(dnsStart)

	metrics1 := collectMetricsWithLatencies(url, method, len(responseBody), len(req.URL.String()), resp.StatusCode, totalResponseTime, tcpHandshakeLatency, dnsLookupLatency)
	metrics.SendMetrics(metrics1, metricsChannel)

	return HttpResponse{
		Body:                string(responseBody),
		StatusCode:          resp.StatusCode,
		URL:                 url,
		Method:              method,
		Duration:            duration,
		TCPHandshakeLatency: tcpHandshakeLatency,
		DNSLookupLatency:    dnsLookupLatency,
	}, nil
}

type HttpResponse struct {
	Body                string
	StatusCode          int
	URL                 string
	Method              string
	Duration            time.Duration
	TCPHandshakeLatency time.Duration
	DNSLookupLatency    time.Duration
}

func collectMetricsWithLatencies(url, method string, bytesReceived, bytesSent, statusCode int, duration, tcpHandshakeLatency, dnsLookupLatency time.Duration) metrics.Metrics {
	key := fmt.Sprintf("%s %s", method, url)

	epMetrics := &metrics.EndpointMetrics{
		URL:                 url,
		Method:              method,
		StatusCodeCounts:    make(map[int]int),
		ResponseTimes:       0,
		TCPHandshakeLatency: 0,
		DNSLookupLatency:    0,
	}

	epMetrics.Requests = 1
	epMetrics.TotalDuration = duration
	epMetrics.TotalResponseTime = duration
	epMetrics.TotalBytesReceived = bytesReceived
	epMetrics.TotalBytesSent = bytesSent
	epMetrics.StatusCodeCounts[statusCode] = 1
	epMetrics.ResponseTimes = duration
	epMetrics.TCPHandshakeLatency = tcpHandshakeLatency
	epMetrics.DNSLookupLatency = dnsLookupLatency

	return metrics.Metrics{EndpointMetricsMap: map[string]*metrics.EndpointMetrics{key: epMetrics}}
}
