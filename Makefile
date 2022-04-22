default: gen_rest_broker gen_rest_consumer

gen_rest_broker:
	./build.sh broker

gen_rest_consumer:
	./build.sh consumer

clean:
	rm -rf gen/*

protoc-gen-gogo:
	go mod download github.com/golang/protobuf
	go build -o $@ github.com/gogo/protobuf/protoc-gen-gogo
