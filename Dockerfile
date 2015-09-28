FROM golang:1.5
RUN go get github.com/sosedoff/docker-gateway
EXPOSE 2377
CMD ["docker-gateway"]