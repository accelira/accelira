package vmhandler

import (
	"testing"

	"github.com/accelira/accelira/metrics"
	"github.com/accelira/accelira/moduleloader"
)

// Creating a VMPool with a valid size and configuration
func TestCreatingVMPoolWithValidSizeAndConfig(t *testing.T) {
	size := 5
	config := &moduleloader.Config{}
	metricsChan := make(chan metrics.Metrics)

	pool, err := NewVMPool(size, config, metricsChan)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if pool == nil {
		t.Fatalf("expected a valid VMPool, got nil")
	}

	if len(pool.pool) != size {
		t.Fatalf("expected pool size %d, got %d", size, len(pool.pool))
	}
}

// Handling a size of zero for the VMPool
func TestHandlingZeroSizeVMPool(t *testing.T) {
	size := 0
	config := &moduleloader.Config{}
	metricsChan := make(chan metrics.Metrics)

	pool, err := NewVMPool(size, config, metricsChan)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if pool == nil {
		t.Fatalf("expected a valid VMPool, got nil")
	}

	if len(pool.pool) != size {
		t.Fatalf("expected pool size %d, got %d", size, len(pool.pool))
	}
}
