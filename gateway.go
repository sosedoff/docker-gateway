package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
)

type Gateway struct {
	Destinations map[string]*Destination
	*sync.Mutex
}

type Destination struct {
	targetUrl *url.URL
	proxy     *httputil.ReverseProxy
}

func NewGateway() *Gateway {
	return &Gateway{
		map[string]*Destination{},
		&sync.Mutex{},
	}
}

func NewDestination(host string, port string) (*Destination, error) {
	targetUrl, err := url.Parse(fmt.Sprintf("http://%v:%v", host, port))
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

func (gw *Gateway) Add(container *docker.Container) error {
	log.Println("Adding container:", container.ID)

	if gw.Destinations[container.ID] != nil {
		return fmt.Errorf("Destination already exists")
	}

	if len(container.Config.ExposedPorts) == 0 {
		log.Printf("Container %s does not have any exposed ports\n", container.ID)
		return nil
	}

	ip := container.NetworkSettings.IPAddress
	port := "5000" // default...

	for k, _ := range container.Config.ExposedPorts {
		port = k.Port()
		break
	}

	dest, err := NewDestination(ip, port)
	if err != nil {
		return err
	}

	gw.Lock()
	defer gw.Unlock()

	gw.Destinations[container.ID] = dest
	return nil
}

func (gw *Gateway) Remove(containerId string) error {
	log.Println("Removing container:", containerId)

	if gw.Destinations[containerId] == nil {
		return nil
	}

	gw.Lock()
	defer gw.Unlock()

	delete(gw.Destinations, containerId)
	return nil
}

func (gw *Gateway) Find(containerId string) *Destination {
	// Lookup destination by short container ID
	if len(containerId) == 12 {
		for k, _ := range gw.Destinations {
			if k[0:12] == containerId {
				return gw.Destinations[k]
			}
		}
	}

	return gw.Destinations[containerId]
}

func (gw *Gateway) Handle(w http.ResponseWriter, r *http.Request) {
	containerId := strings.Split(r.Host, ".")[0]
	destination := gw.Find(containerId)

	log.Printf("Request method=%s host=%s path=%s -> %s\n", r.Method, r.Host, r.RequestURI, destination)

	if destination == nil {
		http.Error(w, "No route", http.StatusBadGateway)
		return
	}

	destination.proxy.ServeHTTP(w, r)
}

func (gw *Gateway) RenderDestinations(w http.ResponseWriter, r *http.Request) {
	data, _ := json.Marshal(gw.Destinations)
	fmt.Fprintf(w, "%s", data)
}

func (gw *Gateway) Start(bind string) error {
	log.Printf("Starting gateway server on http://%s\n", bind)

	http.HandleFunc("/_destinations", gw.RenderDestinations)
	http.HandleFunc("/", gw.Handle)
	return http.ListenAndServe(bind, nil)
}
