#!/bin/bash

# -e causes the script to exit on encountering an error
# -m turns on job management, required for our use of `fg` below.
set -em

# The first arg sets the MODE. There are two MODES:
# 1. "server" mode will launch flow, launch the gateway, and deploy a
# source-hello-world capture. It stays running from that point on until the user
# kills the server. This is useful for interactive testing or authoring tests.
# 2. "run" mode will do the same setup as "server" mode, but will then execute
# the typescript tests with deno. When these finish, the test server will
# shutdown.
MODE="${1:-server}"
echo "MODE: $MODE"

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

function bail() {
    echo "$@" 1>&2
    exit 1
}

TESTDIR="test/tmp"

# Ensure we start with an empty dir, since temporary data plane files will go here.
# Remove it, if it exists already.
if [[ -d "${TESTDIR}" ]]; then
    rm -r ${TESTDIR}
fi
mkdir -p "${TESTDIR}"

# Map to an absolute directory.
export TESTDIR=$(realpath ${TESTDIR})

# `flowctl` commands which interact with the data plane look for *_ADDRESS
# variables, which are created by the temp-data-plane we're about to start.
export BROKER_ADDRESS=unix://localhost${TESTDIR}/gazette.sock
export CONSUMER_ADDRESS=unix://localhost${TESTDIR}/consumer.sock
export GATEWAY_ADDRESS=unix://localhost${TESTDIR}/gateway.sock

export BUILD_ID=test-build-id
export CATALOG_SOURCE="test/acmeCo/source-hello-world.flow.yaml"


echo "TESTDIR setup: ${TESTDIR}"

# Always use the latest development package to verify the mutual integration
# of connectors and the Flow runtime.
# curl -L --proto '=https' --tlsv1.2 -sSf "https://github.com/estuary/flow/releases/download/dev/flow-x86-linux.tar.gz" | tar -zx -C ${TESTDIR}

# Start an empty local data plane within our TESTDIR as a background job.
# --poll so that connectors are polled rather than continuously tailed.
# --sigterm to verify we cleanly tear down the test catalog (otherwise it hangs).
# --tempdir to use our known TESTDIR rather than creating a new temporary directory.
# --unix-sockets to create UDS socket files in TESTDIR in well-known locations.
# ${TESTDIR}/flowctl temp-data-plane \
flowctl temp-data-plane \
    --log.level info \
    --poll \
    --tempdir=${TESTDIR} \
    --unix-sockets \
    &
    # --sigterm \
DATA_PLANE_PID=$!

echo "Data plane launched: ${DATA_PLANE_PID}"

# Start the gateway and point it at the data plane
go run . \
  --gateway-address=${GATEWAY_ADDRESS} \
  --broker-address=${BROKER_ADDRESS} \
  --consumer-address=${CONSUMER_ADDRESS} \
  &
GATEWAY_PID=$!

echo "Gateway launched: ${GATEWAY_PID}"

export GATEWAY_PORT=28318
socat -d -d TCP4-LISTEN:${GATEWAY_PORT},fork UNIX-CONNECT:${TESTDIR}/gateway.sock &
PROXY_PID=$!

# Arrange to stop the background processes on exit.
trap "kill -s SIGTERM ${DATA_PLANE_PID} \
   && kill -s SIGTERM ${GATEWAY_PID} \
   && kill -s SIGTERM ${PROXY_PID} \
   && wait ${DATA_PLANE_PID} \
   && wait ${GATEWAY_PID} \
   && wait ${PROXY_PID}" \
   EXIT

# Build the catalog.
# ${TESTDIR}/flowctl api build \
flowctl api build \
  --directory=${TESTDIR}/builds \
  --build-id=${BUILD_ID} \
  --source=${CATALOG_SOURCE} \
  --ts-package || bail "Build failed."

echo "Build finished"

# Activate the catalog.
# ${TESTDIR}/flowctl api activate \
flowctl api activate \
  --build-id=${BUILD_ID} \
  --all \
  --log.level=info || bail "Activate failed."


echo "Activation finished"

if [ "${MODE}" == "run" ]; then
  # Wait just a bit longer for the shard to boot up.
  sleep 5

  cd client
  npm test
else
  wait
fi
