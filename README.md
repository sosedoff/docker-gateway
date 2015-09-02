# docker-gateway

Stupid simple reverse proxy for Docker

## Overview

Docker gateway project is a simple HTTP reverse proxy that makes containers accessible if
they expose any TCP ports. It will also listen for docker events and bring up / take down hosts
automatically. Gateway project is very similar to [nginx-proxy](https://github.com/jwilder/nginx-proxy) but
implemented in Go and does not require any dependencies.

## Install

With Go:

```
go get github.com/sosedoff/docker-gateway
```

## Development

There are few make tasks available:

- `make setup` - Install dependencies
- `make build` - Build a binary for current environment
- `make all`   - Build binaries for Linux and OSX (amd64 only)
- `make clean` - Remove temp files

## Usage

First, make sure you have Docker running without TLS support. That usually 
mean that it should be running on port `2375` (instead of 2376 for tls).

Then start gateway:

```
DOCKER_HOST=tcp://127.0.0.1:2375 \
GW_DOMAIN=docker.dev \
docker-gateway
```

Gateway will start on `http://0.0.0.0:2377`.

Now, you will need to start a few containers:

```
docker run -d -e DOMAIN=test1.docker.dev sosedoff/dummy-service
docker run -d -e DOMAIN=test2.docker.dev sosedoff/dummy-service
docker run -d -e DOMAIN=test3.docker.dev sosedoff/dummy-service
```

Gateway will automatically map those container to: http://test(1,2,3).docker.dev/

### Using with nginx

```
http {
  upstream docker_gateway_local {
    server 127.0.0.1:2377;
  }

  server {
    listen 80;
    server_name *.docker.dev;

    location / {
      proxy_set_header Host $http_host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_pass http://docker_gateway_local;
      proxy_redirect off;
    }
  }
}
```
