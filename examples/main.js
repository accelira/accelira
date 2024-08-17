const http = require('Accelira/http');
const assert = require('Accelira/assert');
const config = require('Accelira/config');
const group = require('Accelira/group');

config.setIterations(2);
config.setRampUpRate(2);
config.setConcurrentUsers(20);

module.exports = function () {
    group.start("All tested requests", function () {
        const getUrl = "https://jsonplaceholder.typicode.com/todos/1";
        const postUrl = "https://jsonplaceholder.typicode.com/posts";
        const putUrl = "https://jsonplaceholder.typicode.com/posts/1";
        const deleteUrl = "https://jsonplaceholder.typicode.com/posts/1";

        const getResponse = http.get(getUrl).ass

        const postResponse = http.post(postUrl, JSON.stringify({ title: "foo", body: "bar", userId: 1 }));
        assert.equal(postResponse.status, 201);

        const putResponse = http.put(putUrl, JSON.stringify({ id: 1, title: "foo", body: "bar", userId: 1 }));
        assert.equal(putResponse.status, 200);

        const deleteResponse = http.delete(deleteUrl);
        assert.equal(deleteResponse.status, 200);
    });
}

