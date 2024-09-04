package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/accelira/accelira/dashboard"
	"github.com/accelira/accelira/metrics"
	"github.com/accelira/accelira/metricsprocessor"
	"github.com/accelira/accelira/moduleloader"
	"github.com/accelira/accelira/report"
	"github.com/accelira/accelira/util"
	"github.com/accelira/accelira/vmhandler"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/cobra"
)

var (
	metricsReceived  int32
	metricsWaitGroup sync.WaitGroup
)

func main() {
	// Start the real-time monitoring dashboard
	// go startDashboard()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	go func() {
		<-signalChan
		// Perform cleanup actions here before exiting
		printMemoryUsage()
		report.GenerateReport(&metricsprocessor.MetricsMap)
		os.Exit(0)
	}()

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
	go metricsprocessor.GatherMetrics(metricsChannel, &metricsWaitGroup)
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

	report.GenerateReport(&metricsprocessor.MetricsMap)
}

func displayConfig(config *moduleloader.Config) {
	fmt.Printf("Concurrent Users: %d\nIterations: %d\nRamp-up Rate: %d\nDuration: %s\n", config.ConcurrentUsers, config.Iterations, config.RampUpRate, config.Duration)
}

func executeTestScripts(code string, config *moduleloader.Config, metricsChannel chan<- metrics.Metrics) {
	vmPool, err := vmhandler.NewVMPool(config.ConcurrentUsers, config, metricsChannel)
	checkError("Error initializing VM pool\n", err)

	var waitGroup sync.WaitGroup

	// Start the progress bar goroutine
	done := make(chan struct{})
	go func() {
		startTime := time.Now()
		progressBarLength := 50 // Length of the progress bar
		fmt.Printf("\033[?25l") // Hide cursor

		for {
			select {
			case <-done:
				fmt.Printf("\033[?25h") // Show cursor
				return
			default:
				elapsed := time.Since(startTime)
				progress := elapsed.Seconds() / config.Duration.Seconds()
				if progress > 1.0 {
					progress = 1.0
				}
				filledLength := int(progress * float64(progressBarLength))
				bar := fmt.Sprintf(
					"\033[0G\033[32m[%s%s]\033[0m %.2f%% \033[33mElapsed:\033[0m %.2f sec / %.2f sec, \033[34mResponses received:\033[0m %d",
					strings.Repeat("▓", filledLength),
					strings.Repeat("░", progressBarLength-filledLength),
					progress*100,
					elapsed.Seconds(),
					config.Duration.Seconds(),
					atomic.LoadInt32(&metricsReceived),
				)

				// Update the terminal display
				fmt.Print(bar)
				time.Sleep(100 * time.Millisecond) // Update every 50ms
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
	progressBarLength := 50
	fmt.Printf("\033[0G\033[32m[%s]\033[0m 100%% \033[33mElapsed:\033[0m %.2f sec / %.2f sec\n",
		strings.Repeat("▓", progressBarLength),
		config.Duration.Seconds(),
		config.Duration.Seconds(),
	)
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
		metricsprocessor.MetricsMap.Range(func(key, value interface{}) bool {
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
