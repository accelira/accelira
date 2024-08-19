package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/accelira/accelira/metrics"
	"github.com/accelira/accelira/moduleloader"
	"github.com/accelira/accelira/report"
	"github.com/accelira/accelira/util"
	"github.com/accelira/accelira/vmhandler"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/cobra"
)

func main() {
	var rootCommand = &cobra.Command{
		Use:   "accelira",
		Short: "Accelira performance testing tool",
	}

	var runCommand = &cobra.Command{
		Use:   "run [script]",
		Short: "Run a JavaScript test script",
		Args:  cobra.ExactArgs(1),
		Run:   executeScript,
	}

	rootCommand.AddCommand(runCommand)

	if err := rootCommand.Execute(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}
}

func executeScript(cmd *cobra.Command, args []string) {
	scriptPath := args[0]

	// Display logo
	util.DisplayLogo()

	// Build the JavaScript code
	builtCode, err := buildJavaScriptCode(scriptPath)
	if err != nil {
		log.Fatalf("Error building JavaScript: %v", err)
	}

	// Create and configure VM
	vmConfig, err := setupVM(builtCode)
	if err != nil {
		log.Fatalf("Error setting up VM: %v", err)
	}

	displayConfig(vmConfig)

	metricsChannel := make(chan metrics.Metrics, 10000)
	var metricsList []metrics.Metrics
	var metricsMutex sync.Mutex
	var metricsWaitGroup sync.WaitGroup

	// Goroutine to process metrics
	go gatherMetrics(metricsChannel, &metricsList, &metricsMutex, &metricsWaitGroup)
	metricsWaitGroup.Add(1)

	// Run the test scripts
	executeTestScripts(builtCode, vmConfig, metricsChannel)

	// Close channels and wait for all goroutines
	close(metricsChannel)
	metricsWaitGroup.Wait()

	// Generate and display the report
	report.GenerateReport(metricsList)
}

func buildJavaScriptCode(scriptPath string) (string, error) {
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{scriptPath},
		Bundle:      true,
		Format:      api.FormatCommonJS,
		Platform:    api.PlatformNeutral,
		Target:      api.ES2015,
		External: []string{
			"Accelira/http",
			"Accelira/assert",
			"Accelira/config",
			"Accelira/group",
			"jsonwebtoken",
			"crypto",
			"fs",
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

func gatherMetrics(metricsChannel <-chan metrics.Metrics, metricsList *[]metrics.Metrics, metricsMutex *sync.Mutex, metricsWaitGroup *sync.WaitGroup) {
	defer metricsWaitGroup.Done()
	for metric := range metricsChannel {
		metricsMutex.Lock()
		*metricsList = append(*metricsList, metric)
		metricsMutex.Unlock()
	}
}

func displayConfig(config *moduleloader.Config) {
	fmt.Printf("Concurrent Users: %d\n", config.ConcurrentUsers)
	fmt.Printf("Iterations: %d\n", config.Iterations)
	fmt.Printf("Ramp-up Rate: %d\n", config.RampUpRate)
}

func executeTestScripts(code string, config *moduleloader.Config, metricsChannel chan<- metrics.Metrics) {
	var waitGroup sync.WaitGroup
	progressBarWidth := 40

	// Initialize VM pool
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
	os.Stdout.Sync() // Ensure the progress bar is flushed to the output
}
