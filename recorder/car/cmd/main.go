package main

import (
	"flag"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/kodek/tesla"
	"github.com/kodek/tesler/common"
	"github.com/kodek/tesler/recorder/car"
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	glog.Info("Loading config")
	conf := common.LoadConfig()

	// Open Tesla API
	teslaClient, err := car.NewTeslaClientFromConfig(conf)
	if err != nil {
		panic(err)
	}

	poller, err := car.NewPoller(teslaClient)
	if err != nil {
		panic(err)
	}

	eveListener := func(v *tesla.Vehicle) {
		glog.Info("Eve state changed!", spew.Sdump(v))
	}

	poppyListener := func(v *tesla.Vehicle) {
		glog.Info("Poppy state changed!", spew.Sdump(v))
	}
	poller.AddListenerFunc("FOO", eveListener)
	poller.AddListenerFunc("BAR", poppyListener)
	poller.Start()

}
