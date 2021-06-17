#!/bin/bash

set -e

docker build -t dev -f $(pwd)/containers/dev/Dockerfile .
docker run -it \
    --network dev_default \
    -v $(pwd)/tmp/root_ca.crt:/root/.mc/certs/CAs/root_ca.crt \
    -v $(pwd)/tmp/root_ca.crt:/usr/local/share/ca-certificates/root_ca.crt \
    -v "$HOME/code":/root/code \
    -e VAULT_TOKEN \
    dev