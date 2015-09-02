package main

import (
	"log"
	"os"

	docker "github.com/fsouza/go-dockerclient"
)

func getEnvVar(name, defval string) string {
	val := os.Getenv(name)
	if val == "" {
		val = defval
	}
	return val
}

func main() {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		log.Fatalln("Please provide DOCKER_HOST environment variable!")
	}

	client, err := docker.NewClient(host)
	if err != nil {
		log.Fatalln(err)
	}

	gateway := NewGateway()
	if gateway.DefaultDomain == "" {
		log.Fatalln("Please provide GW_DOMAIN environment variable!")
	}

	listener := NewListener(client, gateway)
	listener.Init()
	go listener.Start()

	listenHost := getEnvVar("GW_HOST", "0.0.0.0")
	listenPort := getEnvVar("GW_PORT", "2377")

	err = gateway.Start(listenHost + ":" + listenPort)
	if err != nil {
		log.Fatalln(err)
	}
}
