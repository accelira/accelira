package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/accelira/accelira/metrics"
	"github.com/accelira/accelira/moduleloader"
	"github.com/accelira/accelira/report"
	"github.com/accelira/accelira/util"
	"github.com/accelira/accelira/vmhandler"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/influxdata/tdigest"
	"github.com/spf13/cobra"
)

func main() {
	rootCommand := &cobra.Command{
		Use:   "accelira",
		Short: "Accelira performance testing tool",
	}
	runCommand := &cobra.Command{
		Use:   "run [script]",
		Short: "Run a JavaScript test script",
		Args:  cobra.ExactArgs(1),
		Run:   executeScript,
	}
	rootCommand.AddCommand(runCommand)

	if err := rootCommand.Execute(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	printMemoryUsage()
	printCPUUsage()
}

func printMemoryUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("\nAlloc = %v MiB\tTotalAlloc = %v MiB\tSys = %v MiB\tNumGC = %v\n", bToMb(m.Alloc), bToMb(m.TotalAlloc), bToMb(m.Sys), m.NumGC)
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

func printCPUUsage() {
	fmt.Printf("Number of Goroutines: %d\n", runtime.NumGoroutine())
	userTime := time.Now()
	for i := 0; i < 1000000000; i++ {
	}
	fmt.Printf("Elapsed CPU Time: %s\n", time.Since(userTime))
}

func executeScript(cmd *cobra.Command, args []string) {
	util.DisplayLogo()
	builtCode, err := buildJavaScriptCode(args[0])
	if err != nil {
		log.Fatalf("Error building JavaScript: %v", err)
	}

	vmConfig, err := setupVM(builtCode)
	if err != nil {
		log.Fatalf("Error setting up VM: %v", err)
	}

	displayConfig(vmConfig)
	metricsChannel := make(chan metrics.Metrics, 1000)
	metricsMap := make(map[string]*metrics.EndpointMetrics)
	var metricsMutex sync.Mutex
	var metricsWaitGroup sync.WaitGroup

	go gatherMetrics(metricsChannel, metricsMap, &metricsMutex, &metricsWaitGroup)
	metricsWaitGroup.Add(1)

	executeTestScripts(builtCode, vmConfig, metricsChannel)

	close(metricsChannel)
	metricsWaitGroup.Wait()

	report.GenerateReport1(metricsMap)
}

func buildJavaScriptCode(scriptPath string) (string, error) {
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{scriptPath},
		Bundle:      true,
		Format:      api.FormatCommonJS,
		Platform:    api.PlatformNeutral,
		Target:      api.ES2015,
		External: []string{
			"Accelira/http", "Accelira/assert", "Accelira/config",
			"Accelira/group", "jsonwebtoken", "crypto", "fs",
		},
	})

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("esbuild errors: %v", result.Errors)
	}

	return string(result.OutputFiles[0].Contents), nil
}

func setupVM(code string) (*moduleloader.Config, error) {
	_, config, err := vmhandler.CreateConfigVM(code)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM config: %w", err)
	}
	return config, nil
}

func gatherMetrics(metricsChannel <-chan metrics.Metrics, metricsMap map[string]*metrics.EndpointMetrics, metricsMutex *sync.Mutex, metricsWaitGroup *sync.WaitGroup) {
	defer metricsWaitGroup.Done()
	for metric := range metricsChannel {
		metricsMutex.Lock()
		for key, endpointMetric := range metric.EndpointMetricsMap {
			if existingMetric, exists := metricsMap[key]; exists {
				updateExistingMetric(existingMetric, endpointMetric)
			} else {
				addNewMetric(metricsMap, key, endpointMetric)
			}
		}
		metricsMutex.Unlock()
	}
}

func updateExistingMetric(existingMetric, endpointMetric *metrics.EndpointMetrics) {
	if endpointMetric.Errors > 0 {
		existingMetric.Errors += endpointMetric.Errors
		return
	}

	existingMetric.Requests += endpointMetric.Requests
	existingMetric.TotalDuration += endpointMetric.TotalDuration
	existingMetric.TotalResponseTime += endpointMetric.TotalResponseTime
	existingMetric.TotalBytesReceived += endpointMetric.TotalBytesReceived
	existingMetric.TotalBytesSent += endpointMetric.TotalBytesSent

	for statusCode, count := range endpointMetric.StatusCodeCounts {
		existingMetric.StatusCodeCounts[statusCode] += count
	}

	existingMetric.ResponseTimesTDigest.Add(float64(endpointMetric.ResponseTimes.Milliseconds()), 1)
	existingMetric.TCPHandshakeLatencyTDigest.Add(float64(endpointMetric.TCPHandshakeLatency.Milliseconds()), 1)
	existingMetric.DNSLookupLatencyTDigest.Add(float64(endpointMetric.DNSLookupLatency.Milliseconds()), 1)
}

func addNewMetric(metricsMap map[string]*metrics.EndpointMetrics, key string, endpointMetric *metrics.EndpointMetrics) {
	endpointMetric.ResponseTimesTDigest = tdigest.New()
	endpointMetric.TCPHandshakeLatencyTDigest = tdigest.New()
	endpointMetric.DNSLookupLatencyTDigest = tdigest.New()
	endpointMetric.ResponseTimesTDigest.Add(float64(endpointMetric.ResponseTimes.Milliseconds()), 1)
	endpointMetric.TCPHandshakeLatencyTDigest.Add(float64(endpointMetric.TCPHandshakeLatency.Milliseconds()), 1)
	endpointMetric.DNSLookupLatencyTDigest.Add(float64(endpointMetric.DNSLookupLatency.Milliseconds()), 1)

	metricsMap[key] = endpointMetric
}

func displayConfig(config *moduleloader.Config) {
	fmt.Printf("Concurrent Users: %d\nIterations: %d\nRamp-up Rate: %d\n", config.ConcurrentUsers, config.Iterations, config.RampUpRate)
}

func executeTestScripts(code string, config *moduleloader.Config, metricsChannel chan<- metrics.Metrics) {
	var waitGroup sync.WaitGroup
	progressBarWidth := 40

	vmPool, err := vmhandler.NewVMPool(config.ConcurrentUsers, config, metricsChannel)
	if err != nil {
		log.Fatalf("Error initializing VM pool: %v", err)
	}

	for i := 0; i < config.ConcurrentUsers; i++ {
		displayProgressBar(i, config.ConcurrentUsers, progressBarWidth)

		waitGroup.Add(1)
		go vmhandler.RunScriptWithPool(code, metricsChannel, &waitGroup, config, vmPool)
		if config.RampUpRate > 0 {
			time.Sleep(time.Duration(1000/config.RampUpRate) * time.Millisecond)
		}
	}

	waitGroup.Wait()
}

func displayProgressBar(current, total, width int) {
	percentage := (current + 1) * 100 / total
	filledWidth := percentage * width / 100
	progressBar := fmt.Sprintf("[%s%s] %d%% (Current: %d / Target: %d)",
		util.Repeat('=', filledWidth),
		util.Repeat(' ', width-filledWidth),
		percentage,
		current+1,
		total,
	)

	fmt.Printf("\r%s", progressBar)
	os.Stdout.Sync()
}
