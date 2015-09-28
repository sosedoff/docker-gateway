package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
)

type Gateway struct {
	Client        *docker.Client
	DefaultDomain string
	SkipNoDomain  bool
	Destinations  map[string]DestinationMap
	*sync.Mutex
}

func NewGateway(client *docker.Client) *Gateway {
	return &Gateway{
		Client:        client,
		DefaultDomain: os.Getenv("GW_DOMAIN"),
		SkipNoDomain:  os.Getenv("GW_SKIP_NO_DOMAIN") != "",
		Destinations:  map[string]DestinationMap{},
		Mutex:         &sync.Mutex{},
	}
}

func (gw *Gateway) fetchDomain(c *docker.Container) string {
	for _, v := range c.Config.Env {
		if strings.Contains(v, "DOMAIN=") {
			return strings.Replace(v, "DOMAIN=", "", 1)
		}
	}

	if gw.SkipNoDomain {
		return ""
	}

	return fmt.Sprintf("%s.%s", c.ID[0:12], gw.DefaultDomain)
}

func (gw *Gateway) Add(container *docker.Container) error {
	log.Println("Adding container:", container.ID)

	key := gw.fetchDomain(container)

	if key == "" {
		log.Println("Skipped adding container", container.ID)
		return nil
	}

	if gw.Destinations[key][container.ID] != nil {
		return fmt.Errorf("Destination alreaady exists!")
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

	if gw.Destinations[key] == nil {
		gw.Destinations[key] = DestinationMap{}
	}

	gw.Destinations[key][container.ID] = dest
	return nil
}

func (gw *Gateway) Remove(container *docker.Container) error {
	log.Println("Removing container:", container.ID)
	key := gw.fetchDomain(container)

	if len(gw.Destinations[key]) == 0 {
		return nil
	}

	gw.Lock()
	defer gw.Unlock()

	delete(gw.Destinations[key], container.ID)
	return nil
}

func (gw *Gateway) Find(host string) *Destination {
	if len(gw.Destinations[host]) == 0 {
		return nil
	}

	list := []*Destination{}
	for _, dst := range gw.Destinations[host] {
		list = append(list, dst)
	}

	return list[rand.Intn(len(list))]
}

func (gw *Gateway) Handle(w http.ResponseWriter, r *http.Request) {
	destination := gw.Find(r.Host)

	log.Printf("Request method=%s host=%s path=%s -> %s\n", r.Method, r.Host, r.RequestURI, destination)

	if destination == nil {
		http.Error(w, "Cant find any routes for this host", http.StatusBadGateway)
		return
	}

	destination.proxy.ServeHTTP(w, r)
}

func (gw *Gateway) RenderDestinations(w http.ResponseWriter, r *http.Request) {
	result := map[string][]string{}

	for k, dstMap := range gw.Destinations {
		for _, dst := range dstMap {
			result[k] = append(result[k], dst.String())
		}
	}

	data, _ := json.Marshal(result)
	fmt.Fprintf(w, "%s", data)
}

func (gw *Gateway) RenderLogs(w http.ResponseWriter, r *http.Request) {
	dest := gw.Find(r.Host)

	if dest == nil {
		fmt.Fprintln(w, "Cant find any routes for this host")
		return
	}

	// Determine how many lines of logs we need to fetch
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "3000"
	}

	buff := bytes.NewBuffer([]byte{})

	err := gw.Client.Logs(docker.LogsOptions{
		Container:    dest.containerId,
		Stdout:       true,
		Stderr:       true,
		OutputStream: buff,
		ErrorStream:  buff,
		RawTerminal:  true,
		Tail:         lines,
	})

	if err != nil {
		fmt.Fprintln(w, "Error while fetching logs:", err)
		return
	}

	r.Header.Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "%s", buff.String())
}

func (gw *Gateway) Start(bind string) error {
	log.Printf("Starting gateway server on http://%s\n", bind)

	http.HandleFunc("/_routes", gw.RenderDestinations)
	http.HandleFunc("/_destinations", gw.RenderDestinations)
	http.HandleFunc("/_logs", gw.RenderLogs)

	http.HandleFunc("/", gw.Handle)
	return http.ListenAndServe(bind, nil)
}
