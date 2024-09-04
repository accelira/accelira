import http from 'k6/http';
import { check, sleep } from 'k6';

// Define the options for the load test
export let options = {
    scenarios: {
        load_test: {
            executor: 'ramping-vus', // This scenario uses a ramping VUs executor
            startVUs: 1,             // Start with 1 virtual user
            stages: [
                { duration: '1m', target: 10 }, // Ramp up to 10 VUs over 1 minute
                { duration: '2m', target: 10 }, // Hold 10 VUs for 2 minutes
                { duration: '1m', target: 0 },  // Ramp down to 0 VUs over 1 minute
            ],
            tags: { scenario: 'load_test' }, // Tag to identify this scenario
        },
        stress_test: {
            executor: 'constant-vus', // This scenario uses a constant VUs executor
            vus: 50,                 // Use 50 virtual users
            duration: '3m',          // Run for 3 minutes
            tags: { scenario: 'stress_test' }, // Tag to identify this scenario
        },
    },
};

// Define the main test function
export default function () {
    // URL of the API endpoint to test
    const url = 'https://jsonplaceholder.typicode.com/posts/1';
    
    // Perform an HTTP GET request
    let response = http.get(url);
    
    // Check if the response status is 200 (OK)
    check(response, {
        'is status 200': (r) => r.status === 200,
        'response time <= 200ms': (r) => r.timings.duration <= 200,
    });
    
    // Sleep for a short duration between requests
    sleep(1);
}
