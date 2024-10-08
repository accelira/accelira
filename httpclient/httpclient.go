package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/accelira/accelira/metrics"
)

type HTTPClient struct {
	client     *http.Client
	bufferPool sync.Pool
}

func NewHTTPClient() *HTTPClient {

	transport := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     10 * time.Second,
		DisableKeepAlives:   false,
		MaxIdleConnsPerHost: 100,
		TLSHandshakeTimeout: 10 * time.Second, // Timeout for TLS handshake
		ForceAttemptHTTP2:   true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &HTTPClient{
		client: client,
		bufferPool: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 32*1024) // 32KB buffer
				return &buf
			},
		},
	}
}
func handleRequestError(err error, url, method string, duration time.Duration, metricsChannel chan<- metrics.Metrics) (HttpResponse, error) {
	var statusCode int
	var body string

	switch e := err.(type) {
	case *net.OpError:
		if e.Op == "dial" && e.Err.Error() == "connection refused" {
			body = "Connection refused"
			statusCode = http.StatusServiceUnavailable
		} else {
			body = "Network error: " + e.Error()
			statusCode = http.StatusNetworkAuthenticationRequired
		}
	case net.Error:
		if e.Timeout() {
			body = "Request timed out"
			statusCode = http.StatusRequestTimeout
		} else {
			body = "Network error: " + e.Error()
			statusCode = http.StatusNetworkAuthenticationRequired
		}
	default:
		body = "An unexpected error occurred"
		statusCode = http.StatusInternalServerError
	}

	metrics1 := collectMetricsWithLatencies(url, method, 1, 0, 0, statusCode, duration, 0, 0, 0)
	metrics.SendMetrics(metrics1, metricsChannel)

	return HttpResponse{Body: body, StatusCode: statusCode, URL: url, Method: method, Duration: duration}, nil
}
func (hc *HTTPClient) DoRequest(url, method string, body io.Reader, metricsChannel chan<- metrics.Metrics) (HttpResponse, error) {
	var dnsStart, dnsEnd, connectStart, connectEnd, wroteHeadersTime, wroteRequestTime, gotFirstResponseByteTime, tlsHandshakeStart, tlsHandshakeEnd time.Time
	var bytesSent, bytesReceived int // To track total bytes sent/received

	trace := &httptrace.ClientTrace{
		DNSStart:          func(info httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:           func(info httptrace.DNSDoneInfo) { dnsEnd = time.Now() },
		ConnectStart:      func(network, addr string) { connectStart = time.Now() },
		ConnectDone:       func(network, addr string, err error) { connectEnd = time.Now() },
		TLSHandshakeStart: func() { tlsHandshakeStart = time.Now() },
		TLSHandshakeDone:  func(state tls.ConnectionState, err error) { tlsHandshakeEnd = time.Now() },
		GotFirstResponseByte: func() {
			gotFirstResponseByteTime = time.Now()
		},
		WroteHeaders: func() {
			wroteHeadersTime = time.Now()
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			wroteRequestTime = time.Now()
		},
	}

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(context.Background(), trace), method, url, body)
	if err != nil {
		return handleRequestError(err, url, method, time.Duration(0), metricsChannel)
	}

	req.Header.Set("User-Agent", "Accelira perf testing tool/1.0")

	// Calculate request headers size
	var reqHeadersSize int
	for k, v := range req.Header {
		reqHeadersSize += len(k) + len(v[0]) + 4 // "Key: Value\r\n"
	}
	bytesSent += reqHeadersSize

	startTime := time.Now()
	resp, err := hc.client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		return handleRequestError(err, url, method, duration, metricsChannel)
	}
	defer resp.Body.Close()

	buf := hc.bufferPool.Get().(*[]byte)
	defer hc.bufferPool.Put(buf)

	var responseBody bytes.Buffer
	bytesCopied, err := io.CopyBuffer(&responseBody, resp.Body, *buf)
	if err != nil {
		return HttpResponse{}, err
	}

	// Calculate response headers size
	var respHeadersSize int
	for k, v := range resp.Header {
		respHeadersSize += len(k) + len(v[0]) + 4 // "Key: Value\r\n"
	}
	bytesReceived += respHeadersSize
	bytesReceived += int(bytesCopied) // Add the body size

	// Adding the request body size if any
	if body != nil {
		bodySize, _ := io.Copy(io.Discard, body) // Measure body length
		bytesSent += int(bodySize)
	}

	if tlsHandshakeEnd.Sub(tlsHandshakeStart) > 100*time.Second {
		// Log detailed trace timings
		fmt.Printf("result: %v\n", "============================")
		fmt.Printf("DNS Lookup: %v\n", dnsEnd.Sub(dnsStart))
		fmt.Printf("TCP Connection: %v\n", connectEnd.Sub(connectStart))
		fmt.Printf("TLS Handshake: %v\n", tlsHandshakeEnd.Sub(tlsHandshakeStart))
		fmt.Printf("Time to Write Headers: %v\n", wroteHeadersTime.Sub(connectEnd))
		fmt.Printf("Time to Write Request: %v\n", wroteRequestTime.Sub(wroteHeadersTime))
		fmt.Printf("Time to First Byte: %v\n", gotFirstResponseByteTime.Sub(wroteRequestTime))
	}

	httpResp := HttpResponse{
		Body:                responseBody.String(),
		StatusCode:          resp.StatusCode,
		URL:                 url,
		Method:              method,
		Duration:            duration,
		TCPHandshakeLatency: connectEnd.Sub(connectStart),
		TLSHandshakeLatency: tlsHandshakeEnd.Sub(tlsHandshakeStart),
		DNSLookupLatency:    dnsEnd.Sub(dnsStart),
	}

	// Update metrics with bytes sent/received (including headers)
	metrics1 := collectMetricsWithLatencies(url, method, 0, bytesReceived, bytesSent, resp.StatusCode, duration, httpResp.TCPHandshakeLatency, httpResp.TLSHandshakeLatency, httpResp.DNSLookupLatency)
	metrics.SendMetrics(metrics1, metricsChannel)

	return httpResp, nil
}

func collectMetricsWithLatencies(url, method string, errors int, bytesReceived, bytesSent, statusCode int, duration, tcpHandshakeLatency, tlsHandshakeLatency, dnsLookupLatency time.Duration) metrics.Metrics {
	key := fmt.Sprintf("%s %s", method, url)

	epMetrics := &metrics.EndpointMetrics{
		Type:                metrics.HTTPRequest,
		URL:                 url,
		Method:              method,
		StatusCodeCounts:    map[int]int{statusCode: 1},
		ResponseTime:        duration,
		TCPHandshakeLatency: tcpHandshakeLatency,
		TLSHandshakeLatency: tlsHandshakeLatency,
		DNSLookupLatency:    dnsLookupLatency,
		BytesReceived:       bytesReceived,
		BytesSent:           bytesSent,
		Errors:              errors,
	}

	return metrics.Metrics{EndpointMetricsMap: map[string]*metrics.EndpointMetrics{key: epMetrics}}
}

type HttpResponse struct {
	Body                string
	StatusCode          int
	URL                 string
	Method              string
	Duration            time.Duration
	TCPHandshakeLatency time.Duration
	TLSHandshakeLatency time.Duration
	DNSLookupLatency    time.Duration
}
