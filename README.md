# accelira
**Accelira** is an open-source performance testing tool inspired by K6, designed to simplify and accelerate performance testing for modern applications. By combining the power of Go with the flexibility of JavaScript, Accelira offers an intuitive and efficient way to measure and optimize your application’s performance.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)
- [Contributing](#contributing)

## Features

- **Modern JavaScript Support**: Write performance tests using the latest JavaScript standards with Goja.
- **Efficient Execution**: Leverage Go’s performance and concurrency capabilities for fast and reliable test execution.
- **Easy Integration**: Seamlessly integrate with your CI/CD pipelines and testing workflows.
- **Open Source**: Contribute to and benefit from a collaborative community-driven project.

## Installation

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

4. Verify Installation
```
./accelira --version

```

## Usage 
```
./accelira main.js
```

## Contributing
We welcome contributions from the community! To contribute to Accelira:

- Fork the repository.
- Create a feature branch (`git checkout -b feature-branch`).
- Commit your changes (`git commit -am 'Add new feature`).
- Push to the branch (`git push origin feature-branch`).
- Open a pull request.
