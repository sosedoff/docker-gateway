package main

import (
	"fmt"
	"net/http/httputil"
	"net/url"
	"os"

	docker "github.com/fsouza/go-dockerclient"
)

type DestinationMap map[string]*Destination

type Destination struct {
	containerId string
	targetUrl   *url.URL
	proxy       *httputil.ReverseProxy
}

func getDefaultPort() string {
	port := os.Getenv("DEFAULT_PORT")
	if port == "" {
		// This is a default foreman port
		port = "5000"
	}

	return port
}

func NewDestination(container *docker.Container) (*Destination, error) {
	ip := container.NetworkSettings.IPAddress
	port := getDefaultPort()

	for k, _ := range container.Config.ExposedPorts {
		port = k.Port()
		break
	}

	targetUrl, err := url.Parse(fmt.Sprintf("http://%v:%v", ip, port))
	if err != nil {
		return nil, err
	}

	dest := &Destination{
		container.ID,
		targetUrl,
		httputil.NewSingleHostReverseProxy(targetUrl),
	}

	return dest, nil
}

func (d *Destination) String() string {
	return d.targetUrl.String()
}
