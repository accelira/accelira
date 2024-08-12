const http = require('http');
const assert = require('assert');
const config = require('config');
const group = require('group');


config.setIterations(2); // Example: Set iterations to 2
config.setRampUpRate(2); // Example: Set ramp-up rate to 1 user per second
config.setConcurrentUsers(10); // Example: Set concurrent users to 5

async function main() {
    console.log("Starting script execution...");

    const endGroup = group.start("User Workflow");
    try {
        const getUrl = "https://jsonplaceholder.typicode.com/todos/1";
        const postUrl = "https://jsonplaceholder.typicode.com/posts";
        const putUrl = "https://jsonplaceholder.typicode.com/posts/1";
        const deleteUrl = "https://jsonplaceholder.typicode.com/posts/1";

        const getResponse = await http.get(getUrl);
		console.log(getResponse)
        assert.equal(getResponse.status, 200);

        const postResponse = await http.post(postUrl, JSON.stringify({ title: "foo", body: "bar", userId: 1 }));
        assert.equal(postResponse.status, 201);

        const putResponse = await http.put(putUrl, JSON.stringify({ id: 1, title: "foo", body: "bar", userId: 1 }));
        assert.equal(putResponse.status, 200);

        const deleteResponse = await http.delete(deleteUrl);
        assert.equal(deleteResponse.status, 200);
    } catch (error) {
        console.error("An error occurred within the group:", error);
    } finally {
        endGroup(); // End the group and record the metrics
    }
}

main();