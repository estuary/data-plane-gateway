default: gen_rest_broker gen_rest_consumer

gen_rest_broker:
	./build.sh broker

gen_rest_consumer:
	./build.sh consumer

clean:
	rm -rf gen/*

protobuf_tools:
	go install \
		github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway \
		github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger \
		github.com/golang/protobuf/protoc-gen-go \
		github.com/gogo/protobuf/protoc-gen-gogo
