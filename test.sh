#!/bin/bash

set -e

function bail() {
    log "$@" 1>&2
    exit 1
}

function log() {
  echo "[test.sh] ${1}"
}

# The first arg sets the MODE. There are two MODES:
# 1. "server" mode will launch flow, launch the gateway, and deploy a
# source-hello-world capture. It stays running from that point on until the user
# kills the server. This is useful for interactive testing or authoring tests.
# 2. "run" mode will do the same setup as "server" mode, but will then execute
# the typescript tests with deno. When these finish, the test server will
# shutdown.
MODE="${1:-server}"
log "MODE: $MODE"

# The second arg sets the GATEWAY_BIN. This is the full path to the gateway binary to
# use. This allows local developers to use their local build/installation of
# the gateway, while CI can build a fresh copy immediately before running tests.
GATEWAY_BIN="${2}"
if [ -z "${GATEWAY_BIN}" ]; then
  GATEWAY_BIN=$(which data-plane-gateway)
fi

# The third arg sets the FLOW_BIN. This is the full path to the flow binary to
# use. This allows local developers to use their local build/installation of
# flowctl, while CI can download the flowctl binary to a known location.
FLOW_BIN="${3}"
if [ -z "${FLOW_BIN}" ]; then
  FLOW_BIN=$(which flowctl)
fi

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "${ROOT_DIR}"

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
export GATEWAY_PORT=28318
export GATEWAY_ADDRESS=localhost:${GATEWAY_PORT}

export BUILD_ID=test-build-id
export CATALOG_SOURCE="test/acmeCo/source-hello-world.flow.yaml"

log "TESTDIR setup: ${TESTDIR}"

# Start an empty local data plane within our TESTDIR as a background job.
# --poll so that connectors are polled rather than continuously tailed.
# --sigterm to verify we cleanly tear down the test catalog (otherwise it hangs).
# --tempdir to use our known TESTDIR rather than creating a new temporary directory.
# --unix-sockets to create UDS socket files in TESTDIR in well-known locations.
${FLOW_BIN} temp-data-plane \
  --log.level info \
  --poll \
  --tempdir=${TESTDIR} \
  --unix-sockets \
  --sigterm \
  &
DATA_PLANE_PID=$!

log "Data plane launched: ${DATA_PLANE_PID}"

# Start the gateway and point it at the data plane
${GATEWAY_BIN} \
  --gateway-address=${GATEWAY_ADDRESS} \
  --broker-address=${BROKER_ADDRESS} \
  --consumer-address=${CONSUMER_ADDRESS} \
  &
GATEWAY_PID=$!

log "Gateway launched: ${GATEWAY_PID}"

# Arrange to stop the background processes on exit.
trap "kill -s SIGTERM ${DATA_PLANE_PID} \
   && kill -s SIGTERM ${GATEWAY_PID} \
   && wait ${DATA_PLANE_PID} \
   && wait ${GATEWAY_PID} \
   && exit 0" \
   EXIT

# Build the catalog.
${FLOW_BIN} api build \
  --directory=${TESTDIR}/builds \
  --build-id=${BUILD_ID} \
  --source=${CATALOG_SOURCE} || bail "Build failed."

log "Build finished"

# Activate the catalog.
${FLOW_BIN} api activate \
  --build-id=${BUILD_ID} \
  --all \
  --log.level=info || bail "Activate failed."


log "Activation finished"

if [ "${MODE}" == "run" ]; then
  # Wait just a bit longer for the shard to boot up.
  sleep 5

  make test

  # Tests pass, so let's cleanup the test catalog so the data plane exits cleanly.
  ${FLOW_BIN} api delete \
  --build-id=${BUILD_ID} \
  --all \
  --log.level=info || bail "Delete failed."

  log "Test Passed"
else
  log "Ready"
  wait
fi