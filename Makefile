.PHONY: build test lint proto run certs clean

build:
	go build -o bin/jobworker ./cmd/jobworker

test:
	go test -race ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	buf lint

# Regenerate gen/ from proto/ (needs buf, protoc-gen-go, protoc-gen-go-grpc).
proto:
	buf generate

run: build
	./bin/jobworker serve

certs:
	scripts/gen-certs.sh certs

clean:
	rm -rf bin data
