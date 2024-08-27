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
You can run Uplink Relay in a Docker container. 

Here's how to use the pre-built Docker image:
```
docker run -p 8080:8080 --mount "type=bind,source=./config.yml,target=/app/config.yml" ghcr.io/apollosolutions/uplink-relay:v0.0.1 --config /app/config.yml
```

## Binaries

At the moment, Uplink Relay does not provide prebuilt binaries and will need to be built using the steps under [the developing locally section](#developing-locally).

## Configuration

Uplink Relay can be configured using a YAML configuration file. Here's a complete example:

```yaml
relay:
  address: "localhost:8080"
  publicURL: "http://localhost:8080" # This represents the accessible URL for uplink-relay for use with persisted query manifest fetching.

uplink:
  timeout: 10
  # URLs to use for Uplink. Below are the default values.
  urls:
    - "https://uplink.api.apollographql.com/"
    - "https://aws.uplink.api.apollographql.com/"

# Settings when using an in-memory cache
cache:
  duration: 60 # Cache duration in seconds
  maxSize: 1024

# Settings for using Redis; this will override in-memory caching
redis: 
  enabled: true
  address: "localhost:6379"
  password: ""

supergraphs:
   # Add your graph refs and keys here
  - graphRef: graph@variant 
    apolloKey: service:graph:keyvalue
  - graphRef: "${APOLLO_GRAPH_REF}" # ... or use environment variables using this syntax, where APOLLO_GRAPH_REF represents the environment variable
    apolloKey: "${APOLLO_KEY}"
    # The below options allow for "pinning" or fixing versions of a schema/PQ/entitlement for Uplink Relay
    launchID: abcd
    persistedQueryVersion: abcd
    offlineLicense: abcd.efg.hijk

polling:
  enabled: true
  entitlements: true # Poll for updates to entitlements; default is true
  supergraph: true # Poll for updates to supergraphs; default is true
  persistedQueries: true # Poll for updates to persisted queries; default is false
  interval: 10 # You can use an interval in seconds to poll Uplink
  cronExpressions: # or alternatively use a Cron expression to control the times that it will poll
    - "* * * * *" 

# If you'd like to use the Build Status Notification Webhook: https://www.apollographql.com/docs/graphos/metrics/notifications/build-status-notification/
# Uplink Relay can use it for updates to the schema
webhook: 
  enabled: true
  path: "/webhook"
  secret: "${APOLLO_WEBHOOK_SECRET}"

# Enabling the management API, which is exposed on /graphql by default
# It also has introspection enabled to easily find accessible functionality
managementAPI: 
  enabled: true
  path: /graphql
```

## Developing Locally

1. Clone the repository: `git clone https://github.com/apollosolutions/uplink-relay.git`
3. Install dependencies: `go mod download`
4. Build the project: `go build .`
5. Run the project: `./uplink-relay`

### Developing with Docker

Here's how to build and run the Docker image:
```
docker build -t uplink-relay .
docker run -p 8080:8080 uplink-relay
```