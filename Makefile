TARGETS = darwin/amd64 linux/amd64

build:
	go build -o ./bin/docker-gateway

all:
	gox \
		-osarch="$(TARGETS)" \
		-output="./bin/docker-gateway_{{.OS}}_{{.Arch}}"

setup:
	go get || true

clean:
	rm -rf ./bin/*

docker:
	docker build -t sosedoff/docker-gateway .