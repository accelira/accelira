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

        const getResponse1 = http.get("https://reqres.in/api/users");
        // console.log('Received response', getResponse1);
        const assertions = {
            'is status 200': (response) => {
                // console.log('Checking response status', response.StatusCode);
                return response.StatusCode === 200;
            },
        };


        assert.check(getResponse1, assertions);
    });
}

