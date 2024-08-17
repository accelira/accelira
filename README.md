# Accelira

![GitHub release (latest by date)](https://img.shields.io/github/v/release/accelira/accelira)
![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/accelira/accelira/ci.yml)
![GitHub issues](https://img.shields.io/github/issues/accelira/accelira)
![GitHub pull requests](https://img.shields.io/github/issues-pr/accelira/accelira)
![GitHub last commit](https://img.shields.io/github/last-commit/accelira/accelira)

**Accelira** is the future of web performance testing—powerful, flexible, and built for developers who want results without the hassle. Inspired by the best (looking at you, K6), Accelira harnesses the speed of Go and the familiarity of JavaScript to put performance testing at your fingertips.


![Accelira in Action](demos/demo.svg)


## Why Accelira?

Accelira isn’t just another performance testing tool—it’s the evolution of web testing. Here’s why:
- **Blazing Speed**: Powered by Go for maximum efficiency.
- **JavaScript Familiarity**: Write your tests in JavaScript, no learning curve.
- **Extensibility**: Extend with plugins and custom scripts.
- **Real-Time Insights**: View performance metrics as they happen.
- **Seamless CI/CD Integration**: Perfect for modern development pipelines.

## How Does It Work?
Imagine sending a battalion of virtual users to storm your web application. Accelira does just that, simulating real-world usage scenarios to ensure your app is battle-tested and ready for the big leagues.


## Quick Start
### Get Up and Running in Minutes

1. **Clone the Repository**

   ```bash
   git clone https://github.com/accelira/accelira
   cd accelira
   ```
2. ***Install Dependencies*

```
go mod tidy
```

3. Build the tool

```
go build -o accelira
```

### Your First Test Script
Create test.js and paste this:

```javascript
import http from "Accelira/http";
import config from "Accelira/config";
config.setIterations(10);
config.setConcurrentUsers(5);
export default function () {
    http.get("https://example.com");
}
```

Now, let it rip:

```bash
./accelira run test.js

```

### Command-Line Magic
Accelira’s command-line options are designed to give you superpowers:

- iterations: Run your test multiple times.

Pro tip: Need the full list? Just ask:

```bash
./accelira --help

```


### JavaScript API
Accelira gives you a toolbox of JavaScript functions:

http.get(url, [params]): Fire off a GET request.
http.post(url, body, [params]): Send a POST request.
sleep(duration): Pause your test—because every second counts.
Deep dive into our API docs for all the nitty-gritty.


### Real-World Examples
Skip the theory—see Accelira in action:

```bash
./accelira run examples/load_test.js
```
From basic load tests to complex, multi-endpoint simulations, our examples folder has you covered.


## Contributing
We’re building something amazing, and we need your help. Whether it’s squashing bugs, adding new features, or improving the docs, there’s a place for you here.

1. **Fork it**: Make your copy.
2. **Branch out**: `git checkout -b feature-branch`
3. **Create magic**: Add your changes.
4. **Commit**: `git commit -m 'Add new feature'`
5. **Push it**: `git push origin feature-branch`
6. **Pull request**: Let’s get your work merged.
7. **Check out our [Contributing Guide](CONTRIBUTING.md)** for more details.

## The Road Ahead
We’re not stopping here. Here’s what’s next:

- **Rich Reporting**: Detailed, exportable reports.
- **Distributed Testing**: Scale out your tests across multiple machines.
- **Live Dashboards**: Monitor performance in real-time with a sleek UI.

Stay in the loop with our [Releases page](https://github.com/accelira/accelira/releases).



## The Road Ahead
We’re not stopping here. Here’s what’s next:

- **Rich Reporting**: Detailed, exportable reports.
- **Distributed Testing**: Scale tests across multiple machines.
- **Live Dashboards**: Monitor performance in real-time with a sleek UI.

Stay updated on the latest features by visiting our [Releases page](https://github.com/accelira/accelira/releases).

## FAQ

**Q: What environments does Accelira support?**  
A: Accelira is cross-platform, running smoothly on Windows, macOS, and Linux.

**Q: How can I troubleshoot common errors?**  
A: Check our [Troubleshooting Guide](https://github.com/accelira/accelira/wiki/Troubleshooting).

## Sponsors
Love Accelira? Consider supporting its development. [Become a sponsor](https://github.com/sponsors/accelira) and get your logo featured here!

## Code of Conduct
We expect all contributors to adhere to our [Code of Conduct](CODE_OF_CONDUCT.md).

## Join the Community
Let’s grow together:

- **[Discussions](https://github.com/accelira/accelira/discussions)**: Talk shop, ask questions, or share ideas
- **[Issues](https://github.com/accelira/accelira/issues)**: Report bugs or request features

For more details, visit our [GitHub Pages site](https://accelira.github.io/accelira/).

# Acknowledgments
A big shoutout to K6 and the open-source community for paving the way. We’re standing on the shoulders of giants.
