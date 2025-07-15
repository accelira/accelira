package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"net/http"

	"github.com/accelira/accelira/dashboard"
	"github.com/accelira/accelira/metrics"
	"github.com/accelira/accelira/metricsprocessor"
	"github.com/accelira/accelira/moduleloader"
	"github.com/accelira/accelira/report"
	"github.com/accelira/accelira/util"
	"github.com/accelira/accelira/vmhandler"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	metricsWaitGroup sync.WaitGroup
)

func main() {
	// Start the real-time monitoring dashboard
	// go startDashboard()

	//graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	go func() {
		<-signalChan
		printMemoryUsage()
		os.Exit(0)
	}()

	go func() {
		err := http.ListenAndServe("localhost:6060", nil)
		if err != nil {
			util.GetLogger().Error("pprof server error", zap.Error(err))
		} else {
			util.GetLogger().Info("pprof server started on localhost:6060")
		}
	}()
	rootCmd := createRootCommand()
	if err := rootCmd.Execute(); err != nil {
		util.GetLogger().Fatal("Command execution failed", zap.Error(err))
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
	runCmd := &cobra.Command{
		Use:   "run [script]",
		Short: "Run a JavaScript test script",
		Args:  cobra.ExactArgs(1),
		Run:   executeScript,
	}

	runCmd.Flags().IntP("vus", "u", 0, "Number of virtual users (concurrent users, overrides script config)")
	runCmd.Flags().StringP("duration", "d", "", "Test duration (e.g., 30s, 1m, overrides script config)")
	runCmd.Flags().IntP("rps", "r", 0, "Target requests per second (optional, overrides script config)")
	runCmd.Flags().IntP("iterations", "i", 0, "Number of iterations per user (optional, overrides script config)")

	return runCmd
}

func printMemoryUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	util.GetLogger().Info("Memory usage",
		zap.Uint64("AllocMiB", bToMb(m.Alloc)),
		zap.Uint64("TotalAllocMiB", bToMb(m.TotalAlloc)),
		zap.Uint64("SysMiB", bToMb(m.Sys)),
		zap.Uint64("NumGC", uint64(m.NumGC)),
	)
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
	// Parse CLI flag overrides
	vus, _ := cmd.Flags().GetInt("vus")
	durationStr, _ := cmd.Flags().GetString("duration")
	rps, _ := cmd.Flags().GetInt("rps")
	iterations, _ := cmd.Flags().GetInt("iterations")
	util.DisplayLogo()

	builtCode, err := buildJavaScriptCode(args[0])
	checkError("Error building JavaScript", err)

	vmConfig, err := setupVM(builtCode)
	checkError("Error setting up VM", err)

	// Override config with CLI flags if set
	if vus > 0 {
		vmConfig.ConcurrentUsers = vus
	}
	if durationStr != "" {
		dur, err := time.ParseDuration(durationStr)
		if err == nil {
			vmConfig.Duration = dur
		}
	}
	if rps > 0 {
		// We'll use this in the worker loop
		vmConfig.RPS = rps
	}
	if iterations > 0 {
		vmConfig.Iterations = iterations
	}

	displayConfig(vmConfig)

	metricsChannel := make(chan metrics.Metrics, vmConfig.ConcurrentUsers*5)

	startMetricsCollection(metricsChannel)

	executeTestScripts(builtCode, vmConfig, metricsChannel)

	close(metricsChannel)
	metricsWaitGroup.Wait()

	// report.GenerateReport(&metricsprocessor.MetricsMap)
	metricsMap := metricsprocessor.GetAllAggregatedMetrics()
	reportGenerator := report.NewReportGenerator(&metricsMap)

	// Generate the report
	reportGenerator.GenerateReport()
}

func displayConfig(c *moduleloader.Config) {
	util.GetLogger().Info("Test configuration",
		zap.Int("ConcurrentUsers", c.ConcurrentUsers),
		zap.Int("RampUpRate", c.RampUpRate),
		zap.Duration("Duration", c.Duration),
		zap.Int("RPS", c.RPS),
		zap.Int("Iterations", c.Iterations),
	)
	if c.RPS > 0 {
		util.GetLogger().Info("RPS override set", zap.Int("RPS", c.RPS))
	}
	if c.Iterations > 0 {
		util.GetLogger().Info("Iterations per user override set", zap.Int("Iterations", c.Iterations))
	}
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
				bar :=
					"\033[0G\033[32m[" + strings.Repeat("▓", filledLength) + strings.Repeat("░", progressBarLength-filledLength) + "]\033[0m " +
					fmt.Sprintf("%.2f%% ", progress*100) +
					"\033[33mElapsed:\033[0m " + fmt.Sprintf("%.2f", elapsed.Seconds()) +
					" sec / " + fmt.Sprintf("%.2f", config.Duration.Seconds()) +
					" sec, \033[34mResponses received:\033[0m " + fmt.Sprintf("%d", atomic.LoadInt32(&metricsprocessor.MetricsReceived))

				// Update the terminal display
				fmt.Print(bar)
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
	progressBarLength := 50
	fmt.Printf("\033[0G\033[32m[%s]\033[0m 100%% \033[33mElapsed:\033[0m %.2f sec / %.2f sec\n",
		strings.Repeat("▓", progressBarLength),
		config.Duration.Seconds(),
		config.Duration.Seconds(),
	)
	util.GetLogger().Info("Test execution complete",
		zap.Int("ConcurrentUsers", config.ConcurrentUsers),
		zap.Duration("Duration", config.Duration),
		zap.Int("RPS", config.RPS),
		zap.Int("Iterations", config.Iterations),
		zap.Int32("TotalResponses", atomic.LoadInt32(&metricsprocessor.MetricsReceived)),
	)
}

func checkError(message string, err error) {
	if err != nil {
		util.GetLogger().Fatal(message, zap.Error(err))
	}
}

const htmlContent = dashboard.HtmlContent

func startDashboard() {
	// Serve the dashboard HTML content at the root path
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(htmlContent))
	})

	// Serve the metrics at a different endpoint
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics1 := make(map[string]map[string]interface{})

		// Iterate over the map
		for key, value := range metricsprocessor.GetAllAggregatedMetrics() {
			// Directly use value since it's already of type *metrics.EndpointMetricsAggregated

			metrics1[key] = map[string]interface{}{
				// Uncomment if ResponseTimesTDigest is available
				// "50thPercentileLatency": value.ResponseTimesTDigest.Quantile(0.5),
				// "90thPercentileLatency": value.ResponseTimesTDigest.Quantile(0.9),

				// Use ResponseTimes if it's a valid time.Duration
				"realtimeResponse": value.ResponseTimesTDigest.Quantile(95),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(metrics1); err != nil {
			http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
		}
	})

	// Log the dashboard URL and start the server
	util.GetLogger().Info("Dashboard running", zap.String("url", "http://localhost:8080"))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		util.GetLogger().Fatal("Dashboard server failed", zap.Error(err))
	}
}
