#!/bin/bash

set -e

function module_path {
  go list -f '{{ .Dir }}' -m "$1" | tr '\n' ' '
}

# Which gazette service are we creating the proxy for? broker or consumer?
SERVICE="${1:?must-set-service}"

PROTOC_INC_MODULE_NAMES=(
  "github.com/golang/protobuf"
  "github.com/gogo/protobuf"
  "go.gazette.dev/core"
)

# Find all the full filesystem paths of our protobuf dependencies
for mod in ${PROTOC_INC_MODULE_NAMES[*]}; do
  PROTOC_INC_MODULES+="-I $(module_path ${mod})"
done

# Remove all the old generated files
if [[ -d ./gen/${SERVICE} ]]; then
  rm -rf ./gen/${SERVICE}
fi
mkdir -p gen/${SERVICE}/protocol

# Generate the rest gateway module
protoc \
  -I . \
  ${PROTOC_INC_MODULES} \
  --grpc-gateway_out=gen/${SERVICE}/protocol \
  --grpc-gateway_opt=logtostderr=true \
  --grpc-gateway_opt=paths=import \
  --grpc-gateway_opt=module=go.gazette.dev/core/${SERVICE}/protocol \
  --grpc-gateway_opt=generate_unbound_methods=false \
  --grpc-gateway_opt=grpc_api_configuration=${SERVICE}_service.yaml \
  --grpc-gateway_opt=allow_patch_feature=false \
  --swagger_out=gen/ \
  --swagger_opt=grpc_api_configuration=${SERVICE}_service.yaml \
  --swagger_opt=json_names_for_fields=true \
  --swagger_opt=logtostderr=true \
  ${SERVICE}/protocol/protocol.proto

# The fully qualified module path to the particular gazette service package.
pkgpath="go.gazette.dev/core/${SERVICE}/protocol"
# The original name of the go package
pkg=$(basename ${pkgpath})
# The alias we'll use for the original protobuf package. Mirrors how v2's standalone works.
alias="ext"
# The path of our generated gateway file.
gwfile="gen/${SERVICE}/protocol/protocol.pb.gw.go"

# HACK: grpc-gateway-v1 does not support generating standalone gateway files,
# while v2 does not support gogo-proto. Here we'll follow the pattern from etcd
# and rewrite the generated gateway files to explicitly reference the primary
# protobuf module.
sed -i -E "s#package ${pkg}#package gw#g" "${gwfile}"
sed -i -E "s#import \\(#import \\(${alias} \"${pkgpath}\"#g" "${gwfile}"
sed -i -E "s#([ (])([a-zA-Z0-9_]*(Client|Server|Request)([^(]|$))#\\1${alias}.\\2#g" "${gwfile}"
sed -i -E "s# (New[a-zA-Z0-9_]*Client\\()# ${alias}.\\1#g" "${gwfile}"
go fmt "${gwfile}"

# Generate low-level typescript clients from swagger.
npx swagger-typescript-api \
  -p gen/${SERVICE}/protocol/protocol.swagger.json \
  -o client/src/gen/${SERVICE}/protocol/ \
  -n ${SERVICE}.ts \
  --responses \
  --union-enums
