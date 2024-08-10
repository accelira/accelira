function main() {
    console.log("Starting script execution...");
    var getResponse = http.get("https://jsonplaceholder.typicode.com/todos/1");
    console.log("GET Response:", getResponse);
    
    var postResponse = http.post("https://jsonplaceholder.typicode.com/posts", JSON.stringify({title: "foo", body: "bar", userId: 1}));
    console.log("POST Response:", postResponse);
    
    var putResponse = http.put("https://jsonplaceholder.typicode.com/posts/1", JSON.stringify({id: 1, title: "foo", body: "bar", userId: 1}));
    console.log("PUT Response:", putResponse);
    
    var deleteResponse = http.delete("https://jsonplaceholder.typicode.com/posts/1");
    console.log("DELETE Response:", deleteResponse);
}

main();