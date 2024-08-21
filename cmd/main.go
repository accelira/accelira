package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	_ "net/http/pprof"

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
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	rootCmd := createRootCommand()
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}
	printMemoryUsage()
}

func createRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "accelira",
		Short: "Accelira performance testing tool",
	}
	rootCmd.AddCommand(createRunCommand())
	return rootCmd
}

func createRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run [script]",
		Short: "Run a JavaScript test script",
		Args:  cobra.ExactArgs(1),
		Run:   executeScript,
	}
}

func printMemoryUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("\nAlloc = %v MiB\tTotalAlloc = %v MiB\tSys = %v MiB\tNumGC = %v\n",
		bToMb(m.Alloc), bToMb(m.TotalAlloc), bToMb(m.Sys), m.NumGC)
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

// func gatherMetrics(metricsChannel <-chan metrics.Metrics, metricsMap *sync.Map, metricsMutexMap *sync.Map, metricsWaitGroup *sync.WaitGroup) {
// 	defer metricsWaitGroup.Done()

// 	for metric := range metricsChannel {
// 		for key, endpointMetric := range metric.EndpointMetricsMap {
// 			value, loaded := metricsMap.LoadOrStore(key, &metrics.EndpointMetrics{})
// 			existingMetric := value.(*metrics.EndpointMetrics)

// 			mutexValue, _ := metricsMutexMap.LoadOrStore(key, &sync.Mutex{})
// 			mutex := mutexValue.(*sync.Mutex)

// 			mutex.Lock()
// 			if loaded {
// 				updateMetric(existingMetric, endpointMetric)
// 			} else {
// 				addNewMetric(metricsMap, key, endpointMetric)
// 			}
// 			mutex.Unlock()
// 		}
// 	}
// }

// func gatherMetrics(metricsChannel <-chan metrics.Metrics, metricsMap *sync.Map, metricsMutexMap *sync.Map, metricsWaitGroup *sync.WaitGroup) {
// 	defer metricsWaitGroup.Done()

// 	// Periodically print progress
// 	ticker := time.NewTicker(1 * time.Second)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case metric, ok := <-metricsChannel:
// 			if !ok {
// 				// Channel is closed and drained
// 				return
// 			}

// 			// Increment the counter for metrics received
// 			atomic.AddInt32(&metricsReceived, 1)

// 			for key, endpointMetric := range metric.EndpointMetricsMap {
// 				value, loaded := metricsMap.LoadOrStore(key, &metrics.EndpointMetrics{})
// 				existingMetric := value.(*metrics.EndpointMetrics)

// 				mutexValue, _ := metricsMutexMap.LoadOrStore(key, &sync.Mutex{})
// 				mutex := mutexValue.(*sync.Mutex)

// 				mutex.Lock()
// 				if loaded {
// 					updateMetric(existingMetric, endpointMetric)
// 				} else {
// 					addNewMetric(metricsMap, key, endpointMetric)
// 				}
// 				mutex.Unlock()
// 			}

// 		case <-ticker.C:
// 			// Print progress every second
// 			currentCount := atomic.LoadInt32(&metricsReceived)
// 			fmt.Printf("Metrics received so far: %d\n", currentCount)
// 		}
// 	}
// }

func gatherMetrics(metricsChannel <-chan metrics.Metrics, metricsMap *sync.Map, metricsMutexMap *sync.Map, metricsWaitGroup *sync.WaitGroup) {
	defer metricsWaitGroup.Done()

	// Periodically print progress
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var metricsReceived int32
	var totalTimeElapsed int64
	var tickCount int32

	startTime := time.Now()

	for {
		select {
		case metric, ok := <-metricsChannel:
			if !ok {
				// Channel is closed and drained
				return
			}
			// Increment the counter for metrics received
			atomic.AddInt32(&metricsReceived, 1)
			for key, endpointMetric := range metric.EndpointMetricsMap {
				value, loaded := metricsMap.LoadOrStore(key, &metrics.EndpointMetrics{})
				existingMetric := value.(*metrics.EndpointMetrics)
				mutexValue, _ := metricsMutexMap.LoadOrStore(key, &sync.Mutex{})
				mutex := mutexValue.(*sync.Mutex)
				mutex.Lock()
				if loaded {
					updateMetric(existingMetric, endpointMetric)
				} else {
					addNewMetric(metricsMap, key, endpointMetric)
				}
				mutex.Unlock()
			}
		case <-ticker.C:
			// Print progress every second
			currentCount := atomic.LoadInt32(&metricsReceived)
			elapsed := time.Since(startTime).Seconds()
			atomic.AddInt64(&totalTimeElapsed, int64(elapsed))
			atomic.AddInt32(&tickCount, 1)
			averageDuration := float64(totalTimeElapsed) / float64(tickCount)
			fmt.Printf("Responses received so far: %d | Average latency: %.2f seconds\n", currentCount, averageDuration)
			startTime = time.Now() // Reset start time for next interval
		}
	}
}

func updateMetric(existingMetric, endpointMetric *metrics.EndpointMetrics) {
	if endpointMetric.Errors > 0 {
		existingMetric.Errors += endpointMetric.Errors
		return
	}

	existingMetric.Requests += endpointMetric.Requests
	existingMetric.TotalDuration += endpointMetric.TotalDuration
	existingMetric.TotalResponseTime += endpointMetric.TotalResponseTime
	existingMetric.TotalBytesReceived += endpointMetric.TotalBytesReceived
	existingMetric.TotalBytesSent += endpointMetric.TotalBytesSent

	if existingMetric.StatusCodeCounts == nil {
		existingMetric.StatusCodeCounts = make(map[int]int)
	}

	for statusCode, count := range endpointMetric.StatusCodeCounts {
		existingMetric.StatusCodeCounts[statusCode] += count
	}

	addToTDigest(existingMetric, endpointMetric)
}

func addNewMetric(metricsMap *sync.Map, key string, endpointMetric *metrics.EndpointMetrics) {
	initializeTDigest(endpointMetric)
	metricsMap.Store(key, endpointMetric)
}

func addToTDigest(existingMetric, endpointMetric *metrics.EndpointMetrics) {
	existingMetric.ResponseTimesTDigest.Add(float64(endpointMetric.ResponseTimes.Milliseconds()), 1)
	if endpointMetric.TCPHandshakeLatency.Milliseconds() > 0 {
		existingMetric.TCPHandshakeLatencyTDigest.Add(float64(endpointMetric.TCPHandshakeLatency.Milliseconds()), 1)
	}
	if endpointMetric.DNSLookupLatency.Milliseconds() > 0 {
		existingMetric.DNSLookupLatencyTDigest.Add(float64(endpointMetric.DNSLookupLatency.Milliseconds()), 1)
	}
}

func initializeTDigest(endpointMetric *metrics.EndpointMetrics) {
	endpointMetric.ResponseTimesTDigest = tdigest.New()
	endpointMetric.TCPHandshakeLatencyTDigest = tdigest.New()
	endpointMetric.DNSLookupLatencyTDigest = tdigest.New()

	addToTDigest(endpointMetric, endpointMetric)
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

func startMetricsCollection(metricsChannel chan metrics.Metrics, metricsMap *sync.Map, metricsWaitGroup *sync.WaitGroup) {
	metricsWaitGroup.Add(1)
	metricsMutexMap := &sync.Map{}
	go gatherMetrics(metricsChannel, metricsMap, metricsMutexMap, metricsWaitGroup)
}

func executeScript(cmd *cobra.Command, args []string) {
	util.DisplayLogo()

	builtCode, err := buildJavaScriptCode(args[0])
	checkError("Error building JavaScript", err)

	vmConfig, err := setupVM(builtCode)
	checkError("Error setting up VM", err)

	displayConfig(vmConfig)

	metricsChannel := make(chan metrics.Metrics, vmConfig.ConcurrentUsers*2)
	var metricsMap sync.Map
	var metricsWaitGroup sync.WaitGroup

	startMetricsCollection(metricsChannel, &metricsMap, &metricsWaitGroup)

	executeTestScripts(builtCode, vmConfig, metricsChannel)
	// fmt.Printf("Finished executing api call, now generating report")

	close(metricsChannel)
	metricsWaitGroup.Wait()

	report.GenerateReport1(&metricsMap)
}

func displayConfig(config *moduleloader.Config) {
	fmt.Printf("Concurrent Users: %d\nIterations: %d\nRamp-up Rate: %d\n", config.ConcurrentUsers, config.Iterations, config.RampUpRate)
}

func executeTestScripts(code string, config *moduleloader.Config, metricsChannel chan<- metrics.Metrics) {
	vmPool, err := vmhandler.NewVMPool(config.ConcurrentUsers, config, metricsChannel)
	checkError("Error initializing VM pool\n", err)

	var waitGroup sync.WaitGroup
	// progressBarWidth := 40

	for i := 0; i < config.ConcurrentUsers; i++ {
		// displayProgressBar(i, config.ConcurrentUsers, progressBarWidth)
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

func checkError(message string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", message, err)
	}
}
