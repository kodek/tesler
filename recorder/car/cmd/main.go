package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/cenkalti/backoff"
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
				msgDesc := "Success!"
				if err != nil {
					glog.Errorf("Stopped recording loop for VIN %s: %s", v.Vin, err)
					msgDesc = spew.Sprintf("Error: %v", err)
				}

				message := pushover.NewMessageWithTitle(
					msgDesc,
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

const IDLE_TIME_BEFORE_SLEEP = 5 * time.Minute
const IDLE_SAMPLING_FREQUENCY = 10 * time.Second

func samplesBeforeSleep() int64 {
	return IDLE_TIME_BEFORE_SLEEP.Nanoseconds() / IDLE_SAMPLING_FREQUENCY.Nanoseconds()
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

	// TODO: Encapsulate the idle sample timeout into a class.
	idleSamplesRemaining := samplesBeforeSleep()
	for {
		// fetch data
		data, err := getVehicleData(v)
		if err != nil {
			return err
		}

		// record
		err = this.Database.Insert(context.Background(), *car.NewSnapshot(data))
		if err != nil {
			return errors.Wrap(err, "cannot write data to database")
		}

		activeDelay := getIntervalForActivePolling(data)
		if activeDelay != nil {
			// We should keep monitoring.
			idleSamplesRemaining = samplesBeforeSleep()
			time.Sleep(*activeDelay)
		} else {
			if idleSamplesRemaining <= 0 {
				// THIS was the next run, so let's end.
				glog.Infof("Done monitoring VIN %s.", v.Vin)
				return nil
			}
			// We should stop monitoring after a while.
			idleSamplesRemaining = idleSamplesRemaining - 1
			glog.Infof("Recording ends for car %s in %d samples.", v.Vin, idleSamplesRemaining)
			time.Sleep(IDLE_SAMPLING_FREQUENCY)
		}
	}
}

func getVehicleData(v *tesla.Vehicle) (*tesla.VehicleData, error) {
	onError := func(e error, d time.Duration) {
		glog.Errorf("Error fetching VIN %s. Retrying in (%s): %s\n", v.Vin, common.Round(d, time.Millisecond), e)
	}

	var retVal *tesla.VehicleData
	finalErr := backoff.RetryNotify(func() error {
		var err error
		retVal, err = v.VehicleData()
		return err
	}, backoff.NewExponentialBackOff(), onError)
	return retVal, errors.Wrap(finalErr, fmt.Sprintf("could not fetch vehicle data for %s after multiple tries", v.DisplayName))
}

func getIntervalForActivePolling(data *tesla.VehicleData) *time.Duration {
	// A helper function to convert a Duration literal into a pointer.
	pointerOf := func(d time.Duration) *time.Duration {
		return &d
	}

	shiftState := data.DriveState.ShiftState

	if data.DriveState.Speed > 0 {
		glog.Infof("Car %s is actively moving.", data.Vin)
		return pointerOf(1 * time.Second)
	}
	if shiftState == "R" || shiftState == "D" || shiftState == "N" {
		glog.Infof("Car %s not moving, but in gear.", data.Vin)
		return pointerOf(2 * time.Second)
	}

	chargeState := data.ChargeState.ChargingState
	if chargeState == "Charging" || chargeState == "Starting" {
		glog.Infof("Car %s has active charge state: %s.", data.Vin, chargeState)
		return pointerOf(3 * time.Second)
	}

	if data.VehicleState.SentryMode {
		glog.Infof("Sentry mode enabled for %s.", data.Vin)
		return pointerOf(30 * time.Second)
	}

	if data.VehicleState.CenterDisplayState != 0 {
		glog.Infof("Center display is on for car %s.", data.Vin)
		return pointerOf(10 * time.Second)
	}

	if data.ClimateState.IsClimateOn {
		glog.Infof("Climate is on for car %s.", data.Vin)
		return pointerOf(30 * time.Second)
	}

	glog.Infof("Car %s is not active.", data.Vin)
	return nil
}
