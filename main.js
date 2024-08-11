const http = require('http');
const assert = require('assert');
const config = require('config'); // Import the config module

// Set configuration values
config.setIterations(10); // Example: Set iterations to 20
config.setRampUpRate(3); // Example: Set ramp-up rate to 3 users per second
config.setConcurrentUsers(20); // Example: Set concurrent users to 10

// Retrieve configuration values
const iterations = config.getIterations();
const rampUpRate = config.getRampUpRate();
const concurrentUsers = config.getConcurrentUsers();

async function performHttpRequest(url, method, body = null) {
    let response;
    try {
        switch (method) {
            case 'GET':
                response = await http.get(url);
                break;
            case 'POST':
                response = await http.post(url, body);
                break;
            case 'PUT':
                response = await http.put(url, body);
                break;
            case 'DELETE':
                response = await http.delete(url);
                break;
            default:
                throw new Error('Invalid HTTP method');
        }
        return response;
    } catch (error) {
        console.error(`Error in ${method} request to ${url}:`, error);
    }
}

async function main() {
    console.log("Starting script execution...");
    console.log(`Config - Iterations: ${iterations}, Ramp-Up Rate: ${rampUpRate} users/sec, Concurrent Users: ${concurrentUsers}`);

    const getUrl = "https://jsonplaceholder.typicode.com/todos/1";
    const postUrl = "https://jsonplaceholder.typicode.com/posts";
    const putUrl = "https://jsonplaceholder.typicode.com/posts/1";
    const deleteUrl = "https://jsonplaceholder.typicode.com/posts/1";

    // Perform HTTP requests
    const getResponse = await performHttpRequest(getUrl, 'GET');
    console.log("GET Response:", getResponse);

    const postResponse = await performHttpRequest(postUrl, 'POST', JSON.stringify({ title: "foo", body: "bar", userId: 1 }));
    console.log("POST Response:", postResponse);

    const putResponse = await performHttpRequest(putUrl, 'PUT', JSON.stringify({ id: 1, title: "foo", body: "bar", userId: 1 }));
    console.log("PUT Response:", putResponse);

    const deleteResponse = await performHttpRequest(deleteUrl, 'DELETE');
    console.log("DELETE Response:", deleteResponse);

    // Example assertion
    assert.equal(getResponse.includes('userId'), true);
}

main();
