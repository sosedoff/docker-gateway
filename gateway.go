package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
)

type Gateway struct {
	Destinations map[string]DestinationList
	*sync.Mutex
}

type DestinationList []*Destination

type Destination struct {
	cid       string
	targetUrl *url.URL
	proxy     *httputil.ReverseProxy
}

func NewGateway() *Gateway {
	return &Gateway{
		map[string]DestinationList{},
		&sync.Mutex{},
	}
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
		container.ID,
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

	for _, dst := range gw.Destinations[container.ID] {
		if dst.cid == container.ID {
			return fmt.Errorf("Destination alreaady exists:", dst.String())
		}
	}

	if len(container.Config.ExposedPorts) == 0 {
		log.Printf("Container %s does not have any exposed ports\n", container.ID)
		return nil
	}

	dest, err := NewDestination(container)
	if err != nil {
		return err
	}

	gw.Lock()
	defer gw.Unlock()

	gw.Destinations[container.ID] = append(gw.Destinations[container.ID], dest)
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

func (gw *Gateway) randomDestination(list DestinationList) *Destination {
	if len(list) == 0 {
		return nil
	}

	return list[rand.Intn(len(list))]
}

func (gw *Gateway) Find(containerId string) *Destination {
	// Lookup destination by short container ID
	if len(containerId) == 12 {
		for k, _ := range gw.Destinations {
			if k[0:12] == containerId {
				return gw.randomDestination(gw.Destinations[k])
			}
		}
	}

	return gw.randomDestination(gw.Destinations[containerId])
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
	result := map[string][]string{}

	for k, list := range gw.Destinations {
		for _, dst := range list {
			result[k] = append(result[k], dst.String())
		}
	}

	data, _ := json.Marshal(result)
	fmt.Fprintf(w, "%s", data)
}

func (gw *Gateway) Start(bind string) error {
	log.Printf("Starting gateway server on http://%s\n", bind)

	http.HandleFunc("/_destinations", gw.RenderDestinations)
	http.HandleFunc("/", gw.Handle)
	return http.ListenAndServe(bind, nil)
}
