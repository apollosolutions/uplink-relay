# Uplink Relay

Uplink Relay is a caching reverse-proxy for Apollo Uplink. It's designed to improve the performance and reliability of Apollo Uplink by caching responses and providing local supergraph distribution.

## ⚠️ Disclaimer ⚠️

This project is experimental and is not a fully-supported Apollo Graph project.
We may not respond to issues and pull requests at this time.

## Features

- **Caching**: Uplink Relay caches responses from Apollo Uplink, reducing the egress to Apollo Uplink servers and improving response times.
- **Load Balancing**: Uplink Relay uses a round-robin algorithm to distribute requests evenly across multiple Apollo Uplink instances (GCP/AWS).
- **Configurable**: Uplink Relay allows you to configure various parameters such as cache duration and maximum cache size.
- **Polling**: Uplink Relay supports polling to periodically fetch the supergraph schema from Apollo Uplink. This ensures that the cached supergraph schema is always up-to-date.
- **Webhooks**: Uplink Relay can be configured to listen for webhooks, which can trigger an immediate fetch of the supergraph schema when a change is detected.
- **Uplink Proxy**: Uplink Relay acts as a proxy for retrieving a supergraph schema, reducing the need for direct communication between Apollo Router and Apollo Uplink.

## Getting Started

1. Clone the repository: `git clone https://github.com/apollosolutions/uplink-relay.git`
2. Navigate to the project directory: `cd relay`
3. Install dependencies: `go mod download`
4. Build the project: `go build -o relay .`
5. Run the project: `./relay`

## Configuration

Uplink Relay can be configured using a YAML configuration file. Here's a complete example:

```yaml
relay:
  address: "localhost:8080"
uplink:
  timeout: 10
  urls:
    - "https://uplink.api.apollographql.com/"
    - "https://aws.uplink.api.apollographql.com/"
cache:
  duration: 60 # Cache duration in seconds
  maxSize: 1024
supergraphs:
  graphRefs:
    "${APOLLO_GRAPH_REF}": "${APOLLO_KEY}" # Add your graph refs and keys here, or use environment variables
polling:
  enabled: true
  interval: 10
webhook:
  enabled: true
  path: "/webhook"
  secret: "${APOLLO_WEBHOOK_SECRET}"
```

## Apollo Router Configuration

To use Uplink Relay with Apollo Router, you need to configure the `--apollo-uplink-endpoints` option to point to the Uplink Relay instance. Here's an example:

```bash
router --apollo-uplink-endpoints=http://localhost:8080
```

You can also use the `APOLLO_UPLINK_ENDPOINTS` environment variable:

```bash
export APOLLO_UPLINK_ENDPOINTS=http://localhost:8080
router
```

## Docker
You can also run Uplink Relay in a Docker container. 

Here's how to use the pre-built Docker image:
```
docker run -p 8080:8080 ghcr.io/apollosolutions/uplink-relay:main
```


Here's how to build and run the Docker image:
```
docker build -t uplink-relay .
docker run -p 8080:8080 uplink-relay
```