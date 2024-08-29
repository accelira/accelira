package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

	atomic.AddInt32(&metricsReceived, 1)

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

	metricsChannel := make(chan metrics.Metrics, vmConfig.ConcurrentUsers*2)

	startMetricsCollection(metricsChannel)

	executeTestScripts(builtCode, vmConfig, metricsChannel)

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

	for i := 0; i < config.ConcurrentUsers; i++ {
		waitGroup.Add(1)
		go vmhandler.RunScriptWithPool(code, metricsChannel, &waitGroup, config, vmPool)
		if config.RampUpRate > 0 {
			time.Sleep(time.Duration(1000/config.RampUpRate) * time.Millisecond)
		}
	}

	waitGroup.Wait()
}

func checkError(message string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", message, err)
	}
}

const htmlContent = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Accelira Dashboard</title>
    <style>
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background-color: #e0e5e8;
            color: #333;
            margin: 0;
            padding: 0;
        }
        .container {
            max-width: 1200px;
            margin: 40px auto;
            padding: 20px;
            background-color: white;
            border-radius: 12px;
            box-shadow: 0 4px 12px rgba(0,0,0,0.1);
        }
        h1 {
            font-size: 2.5em;
            margin-top: 0;
            color: #333;
            border-bottom: 2px solid #007bff;
            padding-bottom: 10px;
        }
        #metrics {
            margin-top: 20px;
            white-space: pre-wrap;
            font-family: monospace;
            background-color: #f8f9fa;
            padding: 15px;
            border-radius: 8px;
            box-shadow: 0 2px 6px rgba(0,0,0,0.1);
        }
        #charts {
            margin-top: 30px;
            display: flex;
            flex-wrap: wrap;
            gap: 15px;
        }
        .chart-container {
            flex: 1 1 calc(33% - 30px); /* Adjust the percentage as needed for different numbers of charts */
            min-width: 300px; /* Minimum width for each chart */
            padding: 15px;
            background-color: #ffffff;
            border-radius: 8px;
            box-shadow: 0 2px 6px rgba(0,0,0,0.1);
        }
        canvas {
            width: 100% !important;
            height: 300px !important; /* Adjust height if needed */
        }
        .footer {
            margin-top: 40px;
            text-align: center;
            color: #6c757d;
            font-size: 0.9em;
        }
    </style>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
</head>
<body>
    <div class="container">
        <h1>Accelira Performance Dashboard</h1>
        <div id="metrics">Loading metrics...</div>
        <div id="charts"></div>
        <div class="footer">
            <p>Accelira Dashboard - Real-time Metrics Visualization</p>
        </div>
        <script>
            const charts = {};

            async function fetchMetrics() {
                try {
                    const response = await fetch('/metrics');
                    const data = await response.json();
                    const metricsDiv = document.getElementById('metrics');
                    metricsDiv.textContent = JSON.stringify(data, null, 2);

                    const chartsDiv = document.getElementById('charts');
                    
                    for (let endpoint in data) {
                        const endpointData = data[endpoint];
                        const chartId = "chart-" + endpoint.replace(/[^a-zA-Z0-9]/g, '-');
                        
                        if (!charts[chartId]) {
                            const chartContainer = document.createElement('div');
                            chartContainer.className = 'chart-container';
                            chartContainer.innerHTML = "<h2>" + endpoint + "</h2><canvas id=\"" + chartId + "\" width=\"400\" height=\"200\"></canvas>";
                            chartsDiv.appendChild(chartContainer);

                            const ctx = document.getElementById(chartId).getContext('2d');
                            charts[chartId] = new Chart(ctx, {
                                type: 'line',
                                data: {
                                    labels: [], // Initialize with empty labels
                                    datasets: [
                                        {
                                            label: '50th Percentile Latency (ms)',
                                            data: [],
                                            borderColor: 'rgba(255, 99, 132, 1)',
                                            borderWidth: 2,
                                            fill: false,
                                        },
                                        {
                                            label: '90th Percentile Latency (ms)',
                                            data: [],
                                            borderColor: 'rgba(54, 162, 235, 1)',
                                            borderWidth: 2,
                                            fill: false,
                                        }
                                    ]
                                },
                                options: {
                                    responsive: true,
                                    maintainAspectRatio: false,
                                    scales: {
                                        x: { 
                                            title: { 
                                                display: true, 
                                                text: 'Time' 
                                            },
                                            ticks: {
                                                autoSkip: true,
                                                maxTicksLimit: 10,
                                                maxRotation: 0
                                            }
                                        },
                                        y: { 
                                            title: { 
                                                display: true, 
                                                text: 'Latency (ms)' 
                                            },
                                            beginAtZero: true
                                        }
                                    }
                                }
                            });
                        }
                        
                        const chart = charts[chartId];
                        const now = new Date().toLocaleTimeString(); // Current time as label
                        chart.data.labels.push(now);
                        chart.data.datasets[0].data.push(endpointData['50thPercentileLatency']);
                        chart.data.datasets[1].data.push(endpointData['90thPercentileLatency']);
                        
                        // Data down-sampling if more than 50 points
                        if (chart.data.labels.length > 50) {
                            chart.data.labels = downsample(chart.data.labels, 50);
                            chart.data.datasets[0].data = downsample(chart.data.datasets[0].data, 35);
                            chart.data.datasets[1].data = downsample(chart.data.datasets[1].data, 35);
                        }
                        
                        chart.update();
                    }
                } catch (error) {
                    console.error('Error fetching metrics:', error);
                }
            }

            function downsample(data, maxLength) {
                if (data.length <= maxLength) return data;
                const interval = Math.ceil(data.length / maxLength);
                return data.filter((_, index) => index % interval === 0);
            }

            setInterval(fetchMetrics, 1000); // Refresh every second
        </script>
    </div>
</body>
</html>


`

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
				"50thPercentileLatency": endpointMetrics.ResponseTimesTDigest.Quantile(0.5),
				"90thPercentileLatency": endpointMetrics.ResponseTimesTDigest.Quantile(0.9),
			}
			return true
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics1)
		fmt.Printf("%v", metrics1)
	})

	log.Println("Dashboard running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
