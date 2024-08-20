# Start from the latest golang base image
FROM golang:1.22-bookworm as build

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o /app/uplink-relay . 

# Execution stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates

USER 1001:1001

COPY --from=build /app/uplink-relay /app/uplink-relay

# Add Maintainer Info
LABEL maintainer="Apollo GraphQL Solutions"

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
ENTRYPOINT ["/app/uplink-relay"]
