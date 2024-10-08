import crypto from 'crypto';
import fs from 'fs';
import jwt from 'jsonwebtoken';
import config from "Accelira/config";
import { options } from './options.js';
import http from "Accelira/http";
import group from "Accelira/group";

config.setDuration(options.duration);
config.setRampUpRate(options.rampUpRate);
config.setConcurrentUsers(options.concurrentUsers);

export default function () {

    group.start("get request", function () {

        // Load the private key
        const privateKey = fs.readFileSync('./private.key', 'utf8');

        // Define the payload
        const payload = {
            sub: '1234567890',
            name: 'John Doe',
            admin: true
        };

        // Define the options
        const signOptions = {
            algorithm: 'RS256',
            expiresIn: '1h'
        };

        // Generate the token
        const token = jwt.sign(payload, privateKey, signOptions);

        // Output the token
        console.log(token);


        // const getUrl = "https://jsonplaceholder.typicode.com/todos/1";

        // const getResponse = http.get(getUrl).assertStatus(200)

    })

}