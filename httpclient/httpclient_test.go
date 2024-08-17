package httpclient

import (
	"io"
	"net/http"
	"testing"

	"github.com/accelira/accelira/metrics"
)

// Successful HTTP GET request

func TestSuccessfulHttpGetRequest(t *testing.T) {
	url := "http://example.com"
	method := "GET"
	var body io.Reader = nil
	metricsChan := make(chan metrics.Metrics, 1)

	go func() {
		<-metricsChan
	}()

	response, err := HttpRequest(url, method, body, metricsChan)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, response.StatusCode)
	}

	if response.Body == "" {
		t.Errorf("Expected non-empty body")
	}
}

// Invalid URL format
func TestInvalidUrlFormat(t *testing.T) {
	url := "http//invalid-url"
	method := "GET"
	var body io.Reader = nil
	metricsChan := make(chan metrics.Metrics, 1)

	response, err := HttpRequest(url, method, body, metricsChan)

	if err == nil {
		t.Fatalf("Expected error, got none")
	}

	if response.StatusCode != 0 {
		t.Errorf("Expected status code 0, got %d", response.StatusCode)
	}

	if response.Body != "" {
		t.Errorf("Expected empty body")
	}
}
