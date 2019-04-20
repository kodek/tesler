package car

import (
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/kodek/tesla"
)

var pollInterval = flag.Duration("polling_interval", 10*time.Second, "How often to check for car changes.")

type ListenerFunc func(vehicle *tesla.Vehicle)

type Poller struct {
	tc            *tesla.Client
	vinToListener map[string]ListenerFunc
	vinToStatus   map[string]*tesla.Vehicle
}

func (p *Poller) AddListenerFunc(vin string, listenerFn ListenerFunc) {
	if _, ok := p.vinToListener[vin]; ok {
		glog.Fatal("There's already a listener for VIN", vin)
	}
	p.vinToListener[vin] = listenerFn
}

func NewPoller(tc *tesla.Client) (*Poller, error) {
	p := &Poller{
		tc:            tc,
		vinToListener: make(map[string]ListenerFunc),
		vinToStatus:   make(map[string]*tesla.Vehicle),
	}
	return p, nil
}

func (p *Poller) Start() {
	p.pollOnce()

	ticker := time.NewTicker(*pollInterval)
	for range ticker.C {
		p.pollOnce()
		glog.Info("Sleeping Poller")
	}
}

func (p *Poller) pollOnce() {
	glog.Info("Polling vehicle status")
	vehicles, err := p.tc.Vehicles()
	if err != nil {
		glog.Error("Error while fetching vehicles status.", err)
		return
	}

	for _, v := range vehicles {
		glog.Info("Found vehicle status for vin ", v.Vin)
		prev, _ := p.vinToStatus[v.Vin]
		// update cache
		p.vinToStatus[v.Vin] = v.Vehicle

		if !shouldReport(prev, v.Vehicle) {
			glog.Info("Nothing to report for vehicle vin ", v.Vin)
			continue
		}
		listenerFn, ok := p.vinToListener[v.Vin]
		if !ok {
			glog.Fatal("No listener for car with vin ", v.Vin)
		}
		go listenerFn(v.Vehicle)
	}
}

func shouldReport(prev *tesla.Vehicle, new *tesla.Vehicle) bool {
	if prev == nil {
		// If this is the first time, send a report
		return true
	}
	if new == nil {
		panic("new update should not be nil!")
	}
	prevState := *prev.State
	newState := *new.State
	return prevState != newState
}
