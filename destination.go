package main

import (
	"fmt"
	"net/http/httputil"
	"net/url"

	docker "github.com/fsouza/go-dockerclient"
)

type DestinationMap map[string]*Destination

type Destination struct {
	targetUrl *url.URL
	proxy     *httputil.ReverseProxy
}

func NewDestination(container *docker.Container) (*Destination, error) {
	ip := container.NetworkSettings.IPAddress
	port := "5000" // default foreman port

	for k, _ := range container.Config.ExposedPorts {
		port = k.Port()
		break
	}

	targetUrl, err := url.Parse(fmt.Sprintf("http://%v:%v", ip, port))
	if err != nil {
		return nil, err
	}

	dest := &Destination{
		targetUrl,
		httputil.NewSingleHostReverseProxy(targetUrl),
	}

	return dest, nil
}

func (d *Destination) String() string {
	return d.targetUrl.String()
}
