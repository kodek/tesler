package car

import (
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/kodek/tesla"
)

var pollInterval = flag.Duration("polling_interval", 10*time.Second, "How often to check for car changes.")

type OnVehicleChangeFunc func(v *tesla.Vehicle)

type Poller struct {
	tc              *tesla.Client
	changeStatusFns []OnVehicleChangeFunc
	vinToStatus     map[string]*tesla.Vehicle
}

func (p *Poller) AddVehicleChangeListener(listenerFn OnVehicleChangeFunc) {
	p.changeStatusFns = append(p.changeStatusFns, listenerFn)
}

func NewPoller(tc *tesla.Client) (*Poller, error) {
	p := &Poller{
		tc:              tc,
		vinToStatus:     make(map[string]*tesla.Vehicle),
		changeStatusFns: make([]OnVehicleChangeFunc, 0),
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

		if !statusHasChanged(prev, v.Vehicle) {
			glog.Info("Nothing to report for vehicle vin ", v.Vin)
			continue
		}
		for _, listenerFn := range p.changeStatusFns {
			go listenerFn(v.Vehicle)
		}
	}
}

func statusHasChanged(prev *tesla.Vehicle, new *tesla.Vehicle) bool {
	if prev == nil {
		// Report the first fetch as a change.
		return true
	}
	if new == nil {
		panic("new update should not be nil!")
	}
	prevState := *prev.State
	newState := *new.State
	return prevState != newState
}
