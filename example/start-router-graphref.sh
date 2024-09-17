set -e

# Start the enterprise graph sourcing the graph ref and API key from .env
source .env
export APOLLO_UPLINK_ENDPOINTS=http://localhost:8080

./router \
  --config router.yml \
  --dev
