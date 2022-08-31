default: gen_rest_broker gen_rest_consumer gen_js_client

gen_rest_broker:
	./build.sh broker

gen_rest_consumer:
	./build.sh consumer

gen_js_client:
	deno run --allow-read --allow-env --allow-write --allow-run=npm client/bin/build_package.ts

clean:
	rm -rf gen/* client/dist/* client/src/gen/* test/tmp/*

.PHONY: test
test:
	deno test client/test/ --allow-net --allow-read --allow-write --unstable --unsafely-ignore-certificate-errors

.PHONY: update_snapshots
update_snapshots:
	deno test client/test/ --allow-net --allow-read --allow-write --unstable -- --update


PROTOBUF_TOOLS = \
	protoc-gen-grpc-gateway \
	protoc-gen-swagger \
	protoc-gen-go \
	protoc-gen-gogo

protobuf_tools: $(PROTOBUF_TOOLS)

protoc-gen-grpc-gateway:
	GO111MODULEOFF=true go install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway@v1.16.0

protoc-gen-swagger:
	GO111MODULEOFF=true go install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger@v1.16.0

protoc-gen-go:
	GO111MODULEOFF=true go install github.com/golang/protobuf/protoc-gen-go@v1.5.2

protoc-gen-gogo:
	GO111MODULEOFF=true go install github.com/gogo/protobuf/protoc-gen-gogo@v1.3.2
