package main

import (
	"log"

	docker "github.com/fsouza/go-dockerclient"
)

type Listener struct {
	client   *docker.Client
	chEvents chan *docker.APIEvents
	gw       *Gateway
}

func NewListener(client *docker.Client, gw *Gateway) *Listener {
	return &Listener{
		client,
		make(chan *docker.APIEvents),
		gw,
	}
}

func (l *Listener) Init() {
	l.gw.Flush()

	err := l.gw.Load()
	if err != nil {
		log.Println(err)
	}
}

func (l *Listener) Start() error {
	err := l.client.AddEventListener(l.chEvents)
	if err != nil {
		return err
	}

	for {
		event := <-l.chEvents
		if event == nil {
			continue
		}

		l.handleEvent(event)
	}

	return nil
}

func (l *Listener) handleEvent(event *docker.APIEvents) {
	if event == nil {
		return
	}

	switch event.Status {
	case "start":
		container, err := l.client.InspectContainer(event.ID)
		if err == nil {
			l.gw.Remove(container)
			l.gw.Add(container)
		} else {
			log.Println(err)
		}
	case "stop", "destroy", "kill", "die":
		container, err := l.client.InspectContainer(event.ID)
		if err == nil {
			l.gw.Remove(container)
		} else {
			log.Println(err)
		}
	}
}
