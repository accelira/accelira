package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/accelira/accelira/dashboard"
	"github.com/accelira/accelira/metrics"
	"github.com/accelira/accelira/moduleloader"
	"github.com/accelira/accelira/report"
	"github.com/accelira/accelira/util"
	"github.com/accelira/accelira/vmhandler"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/influxdata/tdigest"
	"github.com/spf13/cobra"
)

var (
	metricsMap       sync.Map
	metricsReceived  int32
	metricsMutexMap  sync.Map
	metricsWaitGroup sync.WaitGroup
)

func main() {
	// Start the real-time monitoring dashboard
	go startDashboard()

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

func gatherMetrics(metricsChannel <-chan metrics.Metrics) {
	defer metricsWaitGroup.Done()

	// Periodically print progress
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// var totalTimeElapsed int64
	// var tickCount int32

	// startTime := time.Now()

	for {
		select {
		case metric, ok := <-metricsChannel:
			if !ok {
				// Channel is closed and drained
				return
			}
			// Increment the counter for metrics received
			for key, endpointMetric := range metric.EndpointMetricsMap {

				value, loaded := metricsMap.LoadOrStore(key, &metrics.EndpointMetrics{})
				existingMetric := value.(*metrics.EndpointMetrics)
				mutexValue, _ := metricsMutexMap.LoadOrStore(key, &sync.Mutex{})
				mutex := mutexValue.(*sync.Mutex)
				mutex.Lock()
				if loaded {
					updateMetric(existingMetric, endpointMetric)
				} else {
					addNewMetric(&metricsMap, key, endpointMetric)
				}
				mutex.Unlock()
			}
			// case <-ticker.C:
			// 	// Print progress every second
			// 	currentCount := atomic.LoadInt32(&metricsReceived)
			// 	elapsed := time.Since(startTime).Seconds()
			// 	atomic.AddInt64(&totalTimeElapsed, int64(elapsed))
			// 	atomic.AddInt32(&tickCount, 1)
			// 	averageDuration := float64(totalTimeElapsed) / float64(tickCount)
			// 	// Move cursor to the line below
			// 	fmt.Print("\033[998;0H") // Move to row 1, column 0 (line below the progress bar)
			// 	fmt.Print("\033[K")      // Clear the line from the cursor to the end of the line
			// 	fmt.Printf("Responses received: %d | Avg latency: %.2f sec", currentCount, averageDuration)

			// 	startTime = time.Now() // Reset start time for next interval
		}
	}
}

func updateMetric(existingMetric, endpointMetric *metrics.EndpointMetrics) {
	if endpointMetric.Errors > 0 {
		existingMetric.Errors += endpointMetric.Errors
		return
	}

	atomic.AddInt32(&metricsReceived, 1)

	existingMetric.Requests += endpointMetric.Requests
	existingMetric.ResponseTimes = endpointMetric.ResponseTimes
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

func startMetricsCollection(metricsChannel chan metrics.Metrics) {
	metricsWaitGroup.Add(1)
	go gatherMetrics(metricsChannel)
}

func executeScript(cmd *cobra.Command, args []string) {
	util.DisplayLogo()

	builtCode, err := buildJavaScriptCode(args[0])
	checkError("Error building JavaScript", err)

	vmConfig, err := setupVM(builtCode)
	checkError("Error setting up VM", err)

	displayConfig(vmConfig)

	metricsChannel := make(chan metrics.Metrics, vmConfig.ConcurrentUsers*5)

	startMetricsCollection(metricsChannel)

	executeTestScripts(builtCode, vmConfig, metricsChannel)

	close(metricsChannel)
	metricsWaitGroup.Wait()

	report.GenerateReport1(&metricsMap)
}

func displayConfig(config *moduleloader.Config) {
	fmt.Printf("Concurrent Users: %d\nIterations: %d\nRamp-up Rate: %d\nDuration: %s\n", config.ConcurrentUsers, config.Iterations, config.RampUpRate, config.Duration)
}

// func executeTestScripts(code string, config *moduleloader.Config, metricsChannel chan<- metrics.Metrics) {
// 	vmPool, err := vmhandler.NewVMPool(config.ConcurrentUsers, config, metricsChannel)
// 	checkError("Error initializing VM pool\n", err)

// 	var waitGroup sync.WaitGroup

// 	for i := 0; i < config.ConcurrentUsers; i++ {
// 		waitGroup.Add(1)
// 		go vmhandler.RunScriptWithPool(code, metricsChannel, &waitGroup, config, vmPool)
// 		if config.RampUpRate > 0 {
// 			time.Sleep(time.Duration(1000/config.RampUpRate) * time.Millisecond)
// 		}
// 	}

// 	waitGroup.Wait()
// }

func executeTestScripts(code string, config *moduleloader.Config, metricsChannel chan<- metrics.Metrics) {
	vmPool, err := vmhandler.NewVMPool(config.ConcurrentUsers, config, metricsChannel)
	checkError("Error initializing VM pool\n", err)

	var waitGroup sync.WaitGroup

	// Start the progress bar goroutine
	done := make(chan struct{})
	go func() {
		startTime := time.Now()
		progressBarLength := 50 // Length of the progress bar

		for {
			select {
			case <-done:
				return
			default:
				elapsed := time.Since(startTime)
				progress := elapsed.Seconds() / config.Duration.Seconds()
				if progress > 1.0 {
					progress = 1.0
				}
				filledLength := int(progress * float64(progressBarLength))
				bar := fmt.Sprintf("[%s%s]", strings.Repeat("=", filledLength), strings.Repeat("-", progressBarLength-filledLength))
				fmt.Print("\033[999;0H") // Move to row 999, column 0, which will be the last line
				fmt.Print("\033[K")      // Clear the line from the cursor to the end of the line
				fmt.Printf("%s Elapsed: %.2f sec / %.2f sec, Responses received %d", bar, elapsed.Seconds(), config.Duration.Seconds(), atomic.LoadInt32(&metricsReceived))

				time.Sleep(100 * time.Millisecond) // Update every 100ms
			}
		}
	}()

	for i := 0; i < config.ConcurrentUsers; i++ {
		waitGroup.Add(1)
		go vmhandler.RunScriptWithPool(code, metricsChannel, &waitGroup, config, vmPool)
		if config.RampUpRate > 0 {
			time.Sleep(time.Duration(1000/config.RampUpRate) * time.Millisecond)
		}
	}

	waitGroup.Wait()
	close(done) // Signal the progress bar goroutine to stop

	// Print final progress
	fmt.Printf("\r[%s] Elapsed: %.2f sec / %.2f sec\n", strings.Repeat("=", 50), config.Duration.Seconds(), config.Duration.Seconds())
}

func checkError(message string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", message, err)
	}
}

const htmlContent = dashboard.HtmlContent

func startDashboard() {
	// Handle requests to the root path with the HTML content
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(htmlContent))
	})

	// Serve metrics at a different endpoint
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics1 := make(map[string]map[string]interface{})
		metricsMap.Range(func(key, value interface{}) bool {
			endpointMetrics := value.(*metrics.EndpointMetrics)
			metrics1[key.(string)] = map[string]interface{}{
				// "50thPercentileLatency": endpointMetrics.ResponseTimesTDigest.Quantile(0.5),
				// "90thPercentileLatency": endpointMetrics.ResponseTimesTDigest.Quantile(0.9),
				"realtimeResponse": endpointMetrics.ResponseTimes.Milliseconds(),
			}
			return true
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics1)
	})

	log.Println("Dashboard running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
