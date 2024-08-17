import http from "Accelira/http";
import config from "Accelira/config";
config.setIterations(10);
config.setConcurrentUsers(5);
export default function () {
    http.get("https://example.com");
}