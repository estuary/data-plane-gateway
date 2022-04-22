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
	deno test client/test/ --allow-net --allow-read --allow-write --unstable

.PHONY: update_snapshots
update_snapshots:
	deno test client/test/ --allow-net --allow-read --allow-write --unstable -- --update

protobuf_tools:
	go install \
		github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway \
		github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger \
		github.com/golang/protobuf/protoc-gen-go \
		github.com/gogo/protobuf/protoc-gen-gogo
