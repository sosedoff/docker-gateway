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
		chunks := strings.Split(v, "=")

		if chunks[0] == "DOMAIN" && len(chunks) > 1 {
			return chunks[1]
		}
	}

	if gw.SkipNoDomain {
		return ""
	}

	return fmt.Sprintf("%s.%s", c.ID[0:12], gw.DefaultDomain)
}

func (gw *Gateway) notFound(w http.ResponseWriter, r *http.Request) {
	routes := []string{}

	for host, _ := range gw.Destinations {
		routes = append(routes, fmt.Sprintf("- http://%s", host))
	}

	msg := "Cant find any routes for this host!\n"

	if len(gw.Destinations) > 0 {
		msg += "\nCheck available URLs:\n"
		msg += strings.Join(routes, "\n")
	}

	http.Error(w, msg, http.StatusBadGateway)
}

func (gw *Gateway) Load() error {
	containers, err := gw.Client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		return err
	}

	for _, c := range containers {
		container, err := gw.Client.InspectContainer(c.ID)
		if err == nil {
			gw.Add(container)
		}
	}

	return nil
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

func (gw *Gateway) RemoveByContainerId(containerId string) {
	gw.Lock()
	defer gw.Unlock()

	for host, dstMap := range gw.Destinations {
		for id := range dstMap {
			// Remove matching container from the route table
			if id == containerId {
				delete(dstMap, id)
			}
		}

		// Remove host from the routing table if there are no destination
		if len(dstMap) == 0 {
			delete(gw.Destinations, host)
		}
	}
}

func (gw *Gateway) Flush() {
	gw.Lock()
	defer gw.Unlock()

	for k := range gw.Destinations {
		delete(gw.Destinations, k)
	}
}

func (gw *Gateway) Find(reqHost string) *Destination {
	host := strings.ToLower(strings.Split(reqHost, ":")[0])

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
		gw.notFound(w, r)
		return
	}

	destination.proxy.ServeHTTP(w, r)
}

func (gw *Gateway) RenderDestinations(w http.ResponseWriter, r *http.Request) {
	result := []string{}
	for host := range gw.Destinations {
		result = append(result, host)
	}
	fmt.Fprintf(w, strings.Join(result, "\n"))
}

func (gw *Gateway) RenderDestinationsJson(w http.ResponseWriter, r *http.Request) {
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
		gw.notFound(w, r)
		return
	}

	// Determine how many lines of logs we need to fetch
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "3000"
	}

	container, err := gw.Client.InspectContainer(dest.containerId)
	if err != nil {
		http.Error(w, "Unable to inspect container:"+dest.containerId, http.StatusBadRequest)
		return
	}

	buff := bytes.NewBuffer([]byte{})

	err = gw.Client.Logs(docker.LogsOptions{
		Container:    dest.containerId,
		Stdout:       true,
		Stderr:       true,
		OutputStream: buff,
		ErrorStream:  buff,
		RawTerminal:  container.Config.Tty,
		Tail:         lines,
	})

	if err != nil {
		fmt.Fprintln(w, "Error while fetching logs:", err)
		return
	}

	r.Header.Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "%s", buff.String())
}

func (gw *Gateway) RenderEnvironment(w http.ResponseWriter, r *http.Request) {
	dest := gw.Find(r.Host)

	if dest == nil {
		gw.notFound(w, r)
		return
	}

	container, err := gw.Client.InspectContainer(dest.containerId)
	if err != nil {
		fmt.Fprintln(w, "Error while inspecting container:", err)
		return
	}

	env := strings.Join(container.Config.Env, "\n")
	r.Header.Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "%s", env)
}

func (gw *Gateway) RenderReset(w http.ResponseWriter, r *http.Request) {
	gw.Flush()

	err := gw.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/_routes", 301)
}

func (gw *Gateway) RenderHelp(w http.ResponseWriter, r *http.Request) {
	help := strings.TrimSpace(`
List of all available system endpoints;

/_help   - Show this message
/_routes - List all available routes
/_reset  - Flush all existing routes and load new ones
/_logs   - Print container logs (for specified host)
/_env    - Print container environment variables (for specified host)

To get logs or environment variable for a container:

http://my-container.domain.com/_logs
http://my-container.domain.com/_logs?lines=100
http://my-container.domain.com/_env
`)

	fmt.Fprintf(w, help)
}

func (gw *Gateway) Start(bind string) error {
	log.Printf("Starting gateway server on http://%s\n", bind)

	http.HandleFunc("/_help", gw.RenderHelp)
	http.HandleFunc("/_routes", gw.RenderDestinations)
	http.HandleFunc("/_routes.json", gw.RenderDestinationsJson)
	http.HandleFunc("/_logs", gw.RenderLogs)
	http.HandleFunc("/_env", gw.RenderEnvironment)
	http.HandleFunc("/_reset", gw.RenderReset)
	http.HandleFunc("/", gw.Handle)

	return http.ListenAndServe(bind, nil)
}
