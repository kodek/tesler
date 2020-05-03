package main

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/gregdel/pushover"
	"github.com/kodek/tesla"
	"github.com/kodek/tesler/common"
	"github.com/kodek/tesler/recorder/car"
	"github.com/kodek/tesler/recorder/databases"
)

func main() {
	_ = flag.Set("logtostderr", "true")
	flag.Parse()

	glog.Info("Loading config")
	conf := common.LoadConfig()

	push := pushover.New(conf.Recorder.Pushover.Token)
	pushUser := pushover.NewRecipient(conf.Recorder.Pushover.User)

	// Open Tesla API
	teslaClient, err := car.NewTeslaClientFromConfig(conf)
	if err != nil {
		panic(err)
	}

	stateMonitor, err := car.NewPollingStateMonitor(teslaClient)
	if err != nil {
		panic(err)
	}

	// Open database
	var database databases.Database
	influxConf := conf.Recorder.InfluxDbConfig
	// TODO: Check that config isn't empty/missing.
	database, err = databases.OpenInfluxDbDatabase(
		influxConf.Address,
		influxConf.Username,
		influxConf.Password,
		influxConf.Database)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			panic(err)
		}
	}()

	pushoverFacade := &PushoverFacade{
		push:      push,
		recipient: pushUser,
	}

	for _, c := range conf.Recorder.Cars {
		recorder, err := NewRecorder(database)
		if err != nil {
			panic(err)
		}
		stateMonitor.AddVehicleChangeListener(
			newFilterByCarMiddleware(
				c.Vin,
				c.Monitor,
				newCountStateChangesMiddleware(
					newGreetOnFirstChangeMiddleware(pushoverFacade,
						newRecordMetricsMiddleware(pushoverFacade,
							recorder, newLogAndNotifyMiddleware(pushoverFacade, noOpHandler()))))))
	}

	mux := common.NewKodekMux("Tesler-Recorder-v2")

	defaultHandlerFunc := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/statusz", http.StatusSeeOther)
	}
	mux.HandleFunc("/", defaultHandlerFunc)
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		conf.WriteRedacted(w)
	})
	if conf.Recorder.Port == 0 {
		glog.Fatal("Port 0 currently not supported. Please set config.Recorder.Port to continue.")
	}
	listenSpec := fmt.Sprintf(":%d", conf.Recorder.Port)
	glog.Infof("Starting Tesler recorder server at %s", listenSpec)

	go stateMonitor.Poll()
	glog.Fatal(http.ListenAndServe(listenSpec, mux))
}

func noOpHandler() car.OnVehicleChangeFunc {
	return func(v *tesla.Vehicle) {}
}

func newLogAndNotifyMiddleware(pushoverFacade *PushoverFacade, in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
	return func(v *tesla.Vehicle) {
		defer in(v)
		glog.Infof("Vehicle %s state changed: %s", v.DisplayName, spew.Sdump(v))

		_, err := pushoverFacade.SendMessageWithTitle(
			spew.Sdump(v),
			fmt.Sprintf("Vehicle %s state changed to %s", v.DisplayName, stateString(v)))

		if err != nil {
			glog.Errorf("Cannot send Pushover message: %s", err)
		}
	}
}

func newRecordMetricsMiddleware(pushoverFacade *PushoverFacade, recorder *Recorder, in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
	return func(v *tesla.Vehicle) {
		defer in(v)
		if v.State == nil || *v.State != "online" {
			glog.Infof("Not recording metrics for %s because it's not online.", v.DisplayName)
			return
		}
		// TODO: This needs to be done async because it's inside an Adapter. This should be changed because it's not
		// intuitive. Blocking should be okay, but it shouldn't block the actual listener.
		go func() {
			err := recorder.RecordWhileVehicleInUse(v)
			msgDesc := "Success!"
			if err != nil {
				glog.Errorf("Stopped recording loop for VIN %s: %s", v.Vin, err)
				msgDesc = spew.Sprintf("Error: %v", err)
			}

			_, err = pushoverFacade.SendMessageWithTitle(
				msgDesc,
				fmt.Sprintf("Done monitoring: %s", v.DisplayName))
			if err != nil {
				glog.Errorf("Cannot send Pushover message: %s", err)
			}
		}()
	}
}

func newGreetOnFirstChangeMiddleware(pushoverSender *PushoverFacade, in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
	// NOTE: Not thread-safe.
	isFirst := true
	return func(v *tesla.Vehicle) {
		defer in(v)
		if isFirst {
			isFirst = false

			_, err := pushoverSender.SendMessageWithTitle(
				fmt.Sprintf("Car's state: %s", stateString(v)),
				fmt.Sprintf("Monitoring for %s is ready!", v.DisplayName))
			if err != nil {
				glog.Errorf("Cannot send Pushover message: %s", err)
			}
		}
	}
}

func newCountStateChangesMiddleware(in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
	// NOTE: Not thread-safe.
	count := 0
	return func(v *tesla.Vehicle) {
		defer in(v)
		count = count + 1
		glog.Infof("Count for %s is %d.", v.DisplayName, count)
	}
}

func newFilterByCarMiddleware(vin string, monitor bool, in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
	return func(v *tesla.Vehicle) {
		if v.Vin != vin {
			// Skip the car. Hopefully some other handler will match it.
			// TODO: Restructure code so that if a new vin shows up (outside the config), an error is logged.
			return
		}
		if !monitor {
			glog.Infof("Ignored update for VIN %s. Monitoring disabled in config.", v.Vin)
			return
		}
		in(v)
	}
}

func stateString(v *tesla.Vehicle) string {
	if v.State != nil {
		return *v.State
	}
	return "<unknown>"
}
