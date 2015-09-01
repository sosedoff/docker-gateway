build:
	go build -o ./bin/docker-gateway

setup:
	go get || true

clean:
	rm -rf ./bin/*