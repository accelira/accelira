import http from 'k6/http';
import { check, sleep } from 'k6';

export default function () {
    // Make a GET request to the API
    let response = http.get('https://reqres.in/api/users');

    // Check the response status
    check(response, {
        'is status 200': (r) => r.status === 200,
    });

    // Optional: Wait for a short period to simulate user think time
}
