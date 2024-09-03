import http from "Accelira/http";
import assert from "Accelira/assert";
import config from "Accelira/config";
import group from "Accelira/group";
import { options } from './options.js';


config.setIterations(options.iterations);
config.setRampUpRate(options.rampUpRate);
config.setConcurrentUsers(options.concurrentUsers);
config.setDuration(options.duration);


export default function () {

    group.start("reqres website", function () {
        const getUrl = "https://jsonplaceholder.typicode.com/todos/1";
        const postUrl = "https://jsonplaceholder.typicode.com/posts";
        const putUrl = "https://jsonplaceholder.typicode.com/posts/1";
        const deleteUrl = "https://jsonplaceholder.typicode.com/posts/1";

        // const getResponse = http.get("https://www.google.com");
        // getResponse.assertStatus(429)

        const getResponse1 = http.get("https://reqres.in/api/users");
        getResponse1.assertStatus(200)

        // const putResponse = http.put(putUrl, JSON.stringify({ id: 1, title: "foo", body: "bar", userId: 1 }));
        // putResponse.assertStatus(200)

        // const deleteResponse = http.delete(deleteUrl);
        // deleteResponse.assertStatus(200)
    });
}

