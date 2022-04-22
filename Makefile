default: gen_rest_broker gen_rest_consumer gen_js_client

gen_rest_broker:
	./build.sh broker

gen_rest_consumer:
	./build.sh consumer

gen_js_client:
	deno bundle client/ts/src/index.ts client/js/client.js

clean:
	rm -rf gen/* client/js/* test/tmp/*

test:
	./test.sh

protoc-gen-gogo:
	go mod download github.com/golang/protobuf
	go build -o $@ github.com/gogo/protobuf/protoc-gen-gogo
