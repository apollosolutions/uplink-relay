# Uplink Relay

Uplink Relay is a caching reverse-proxy for Apollo Uplink. It's designed to improve the performance and reliability of Apollo Uplink by caching responses and providing local supergraph distribution.

## ⚠️ Disclaimer ⚠️

This project is experimental and is not a fully-supported Apollo Graph project.
We may not respond to issues and pull requests at this time.

## Features

- **Caching**: Uplink Relay caches responses from Apollo Uplink, reducing the egress to Apollo Uplink servers and improving response times.
- **Load Balancing**: Uplink Relay uses a round-robin algorithm to distribute requests evenly across multiple Apollo Uplink instances (GCP/AWS).
- **Configurable**: Uplink Relay allows you to configure various parameters such as cache duration and maximum cache size.

## Getting Started

1. Clone the repository: `git clone https://github.com/apollosolutions/uplink-relay.git`
2. Navigate to the project directory: `cd relay`
3. Install dependencies: `go mod download`
4. Build the project: `go build -o relay .`
5. Run the project: `./relay`

## Configuration

Uplink Relay can be configured using a YAML configuration file. Here's an example:

```yaml
relay:
  address: "localhost:8080"
uplink:
  timeout: 10
  retryCount: 5
  urls:
    - "https://uplink.api.apollographql.com/"
    - "https://aws.uplink.api.apollographql.com/"
cache:
  duration: 60 # Cache duration in seconds
  maxSize: 1024
```

## Docker
You can also run Uplink Relay in a Docker container. Here's how to build and run the Docker image:
```
docker build -t uplink-relay .
docker run -p 8080:8080 uplink-relay
```
