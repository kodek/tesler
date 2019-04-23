package main

import (
	"context"
	"flag"
	"fmt"
	"time"

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

	poller, err := car.NewPoller(teslaClient)
	if err != nil {
		panic(err)
	}

	newCountAdapter := func(in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
		// NOTE: Not thread-safe.
		count := 0
		return func(v *tesla.Vehicle) {
			defer in(v)
			count = count + 1
			glog.Infof("Count for %s is %d.", v.DisplayName, count)
		}
	}

	greetOnFirstNotificationFn := func(in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
		// NOTE: Not thread-safe.
		isFirst := true
		return func(v *tesla.Vehicle) {
			defer in(v)
			if isFirst {
				isFirst = false

				message := pushover.NewMessageWithTitle(
					fmt.Sprintf("Car's state: %s", stateString(v)),
					fmt.Sprintf("Monitoring for %s is ready!", v.DisplayName))
				_, err := push.SendMessage(message, pushUser)
				if err != nil {
					glog.Errorf("Cannot send Pushover message: %s", err)
				}
			}
		}
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
	defer database.Close()

	recordMetricsAdapter := func(recorder *Recorder, in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
		return func(v *tesla.Vehicle) {
			defer in(v)
			if v.State == nil || *v.State != "online" {
				glog.Infof("Not recording metrics for %s because it's not online.", v.DisplayName)
				return
			}
			// TODO: This needs to be done async because it's inside an Adapter. This should be changed because it's not
			// intuitive. Blocking should be okay, but it shouldn't block the actual listener.
			go func() {
				err := recorder.StartAndBlock(v)
				if err != nil {
					glog.Errorf("Stopped recording loop for VIN %s: %s", v.Vin, err)
				}

				message := pushover.NewMessageWithTitle(
					spew.Sprintf("Error: %v", err),
					fmt.Sprintf("Done monitoring: %s", v.DisplayName))
				_, err = push.SendMessage(message, pushUser)
				if err != nil {
					glog.Errorf("Cannot send Pushover message: %s", err)
				}
			}()
		}
	}
	logAndNotifyListener := func(v *tesla.Vehicle) {
		glog.Infof("Vehicle %s state changed: %s", v.DisplayName, spew.Sdump(v))

		message := pushover.NewMessageWithTitle(
			spew.Sdump(v),
			fmt.Sprintf("Vehicle %s state changed to %s", v.DisplayName, stateString(v)))
		_, err := push.SendMessage(message, pushUser)

		if err != nil {
			glog.Errorf("Cannot send Pushover message: %s", err)
		}
	}

	for _, c := range conf.Recorder.Cars {
		newSingleCarFilter := func(vin string, monitor bool, in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
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

		recorder := &Recorder{
			Database: database,
		}
		poller.AddVehicleChangeListener(
			newSingleCarFilter(
				c.Vin,
				c.Monitor,
				newCountAdapter(
					greetOnFirstNotificationFn(
						recordMetricsAdapter(
							recorder, logAndNotifyListener)))))
	}
	poller.Start()
}

func stateString(v *tesla.Vehicle) string {
	if v.State != nil {
		return *v.State
	}
	return "<unknown>"
}

type Recorder struct {
	recording bool
	Database  databases.Database
}

func (this *Recorder) StartAndBlock(v *tesla.Vehicle) error {
	if this.recording {
		glog.Fatalf("Recorder not reentrant (car VIN %s).", v.Vin)
	}
	// Make function non-reentrant.
	// TODO: Determine if thread safety is required. This function is not thread-safe.
	this.recording = true
	defer func() {
		this.recording = false
	}()

	endIfStillParked := false
	for {
		// fetch data
		data, err := v.VehicleData()
		if err != nil {
			// TODO: Add exponential backoff.
			// TODO: Rewrap error and avoid over-logging.
			glog.Errorf("Cannot fetch vehicle data for %s: %s", v.DisplayName, err)
			return err
		}

		// record
		err = this.Database.Insert(context.Background(), *car.NewSnapshot(data))
		if err != nil {
			glog.Errorf("Cannot fetch vehicle data for %s: %s", v.DisplayName, err)
			return err
		}

		if shouldFastMonitor(data) {
			// We should keep monitoring.
			endIfStillParked = false
			time.Sleep(2 * time.Second)
		} else {
			if endIfStillParked {
				// THIS was the next run, so let's end.
				glog.Infof("Done monitoring VIN %s.", v.Vin)
				return nil
			}
			// We should stop monitoring after a while.
			endIfStillParked = true
			time.Sleep(5 * time.Minute)
		}
	}
}

func shouldFastMonitor(data *tesla.VehicleData) bool {
	shiftState := data.DriveState.ShiftState

	if data.DriveState.Speed > 0 {
		glog.Info("Car %s is actively moving.", data.Vin)
		return true
	}
	if shiftState == "R" || shiftState == "D" || shiftState == "N" {
		glog.Infof("Car %s not moving, but in gear.", data.Vin)
		return true
	}

	chargeState := data.ChargeState.ChargingState
	if chargeState == "Charging" || chargeState == "Starting" {
		glog.Infof("Car %s has active charge state: %s.", data.Vin, chargeState)
		return true
	}
	glog.Infof("Car %s is not active.", data.Vin)
	return false
}
