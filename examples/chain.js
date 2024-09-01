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

    group.start("All tested requests", function () {
        const getUrl = "https://jsonplaceholder.typicode.com/todos/1";
        const postUrl = "https://jsonplaceholder.typicode.com/posts";
        const putUrl = "https://jsonplaceholder.typicode.com/posts/1";
        const deleteUrl = "https://jsonplaceholder.typicode.com/posts/1";

        http.get(getUrl)
            .assertStatus(200);

        http.post(postUrl, JSON.stringify({ title: "foo", body: "bar", userId: 1 }))
            .assertStatus(201);

        http.put(putUrl, JSON.stringify({ id: 1, title: "foo", body: "bar", userId: 1 }))
            .assertStatus(200);

        http.delete(deleteUrl)
            .assertStatus(200);
    });
}