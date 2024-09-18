set -e

echo "Downloading latest Router version..."
curl -sSL https://router.apollo.dev/download/nix/latest | sh

echo "Updating config schema file..."
./router config schema > configuration_schema.json

echo "Success!"
