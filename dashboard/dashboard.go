package dashboard

// HTML content as a raw string
const HtmlContent = `
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
        <div id="charts"></div>
        <div id="metrics">Loading metrics...</div>
        <div class="footer">
            <p>Accelira Dashboard - Real-time Metrics Visualization</p>
        </div>
        <script>
            const charts = {};

            async function fetchMetrics() {
                try {
                    const response = await fetch('/metrics');
                    if (!response.ok) {
                        throw new Error('Failed to fetch metrics');
                    }
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
                                            label: 'Real-time Response (ms)',
                                            data: [],
                                            borderColor: 'rgba(75, 192, 192, 1)',
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
                        chart.data.datasets[0].data.push(endpointData['realtimeResponse']);
                        
                        // Data down-sampling if more than 50 points
                        if (chart.data.labels.length > 50) {
                            chart.data.labels = downsample(chart.data.labels, 50);
                            chart.data.datasets[0].data = downsample(chart.data.datasets[0].data, 50);
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

            const intervalId = setInterval(() => {
                try {
                    fetchMetrics();
                } catch (error) {
                    console.error('An error occurred:', error);
                    clearInterval(intervalId); // Stop the interval if an error occurs
                }
            }, 1000);
        </script>
    </div>
</body>
</html>
`
