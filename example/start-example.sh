docker run -p 8080:8080 \
  --mount "type=bind,source=./tempcache,target=/app/tempcache" \
  --mount "type=bind,source=./example-config.yml,target=/app/config.yml" ghcr.io/apollosolutions/uplink-relay:latest \
  --config /app/config.yml
