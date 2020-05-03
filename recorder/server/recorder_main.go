package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
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

	stateMonitor, err := car.NewPollingStateMonitor(teslaClient)
	if err != nil {
		panic(err)
	}

	countFn := func(in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
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
	defer func() {
		if err := database.Close(); err != nil {
			panic(err)
		}
	}()

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
				err := recorder.RecordWhileVehicleInUse(v)
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
		singleCarFilterFn := func(vin string, monitor bool, in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
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
		stateMonitor.AddVehicleChangeListener(
			singleCarFilterFn(
				c.Vin,
				c.Monitor,
				countFn(
					greetOnFirstNotificationFn(
						recordMetricsAdapter(
							recorder, logAndNotifyListener)))))
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

const IdleTimeBeforeSleep = 5 * time.Minute
const IdleSamplingFrequency = 10 * time.Second

func samplesBeforeSleep() int64 {
	return IdleTimeBeforeSleep.Nanoseconds() / IdleSamplingFrequency.Nanoseconds()
}

func (r *Recorder) RecordWhileVehicleInUse(v *tesla.Vehicle) error {
	if r.recording {
		return errors.New(fmt.Sprintf("Recorder not reentrant (car VIN %s).", v.Vin))
	}
	// Make function non-reentrant.
	// TODO: Determine if thread safety is required. This function is not thread-safe.
	r.recording = true
	defer func() {
		r.recording = false
	}()

	// TODO: Encapsulate the idle sample timeout into a class.
	idleSamplesRemaining := samplesBeforeSleep()
	for {
		// Fetch data.
		data, err := getVehicleData(v)
		if err != nil {
			return err
		}

		// Parse data and active state.
		activeState := newActiveState(data)

		snapshot := car.NewSnapshot(data)
		snapshot.ActiveDescription = activeState.Description()

		// Record.
		err = r.Database.Insert(context.Background(), *snapshot)
		if err != nil {
			return errors.Wrap(err, "cannot write data to database")
		}

		// Determine polling frequency.
		if !activeState.ShouldSleep() {
			// We should keep monitoring.
			idleSamplesRemaining = samplesBeforeSleep()
			time.Sleep(activeState.PollInterval())
		} else {
			if idleSamplesRemaining <= 0 {
				// THIS was the next run, so let's end.
				glog.Infof("Done monitoring VIN %s.", v.Vin)
				return nil
			}
			// We should stop monitoring after a while.
			idleSamplesRemaining = idleSamplesRemaining - 1
			glog.Infof("Recording ends for car %s in %d samples.", v.Vin, idleSamplesRemaining)
			time.Sleep(activeState.PollInterval())
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

type ActiveState struct {
	shouldSleep bool
	pollFreq    time.Duration
	desc        string
}

func (s *ActiveState) PollInterval() time.Duration {
	return s.pollFreq
}

func (s *ActiveState) Description() string {
	return s.desc
}

func (s *ActiveState) ShouldSleep() bool {
	return s.shouldSleep
}

func newActiveState(data *tesla.VehicleData) ActiveState {
	shiftState := data.DriveState.ShiftState

	if data.DriveState.Speed > 0 {
		glog.Infof("Car %s is actively moving.", data.Vin)
		return ActiveState{
			pollFreq: 1 * time.Second,
			desc:     "Moving",
		}
	}
	if shiftState == "R" || shiftState == "D" || shiftState == "N" {
		glog.Infof("Car %s not moving, but in gear.", data.Vin)
		return ActiveState{
			pollFreq: 2 * time.Second,
			desc:     "In gear",
		}
	}

	chargeState := data.ChargeState.ChargingState
	if chargeState == "Charging" || chargeState == "Starting" {
		glog.Infof("Car %s has active charge state: %s.", data.Vin, chargeState)
		return ActiveState{
			pollFreq: 3 * time.Second,
			desc:     "Charging",
		}
	}

	if data.VehicleState.SentryMode {
		glog.Infof("Sentry mode enabled for %s.", data.Vin)
		return ActiveState{
			pollFreq: 30 * time.Second,
			desc:     "Sentry mode",
		}
	}

	if data.VehicleState.CenterDisplayState != 0 {
		glog.Infof("Center display is on for car %s.", data.Vin)
		return ActiveState{
			pollFreq: 10 * time.Second,
			desc:     "Display on",
		}
	}

	if data.ClimateState.IsClimateOn {
		glog.Infof("Climate is on for car %s.", data.Vin)
		return ActiveState{
			pollFreq: 30 * time.Second,
			desc:     "Climate on",
		}
	}

	glog.Infof("Car %s is not active.", data.Vin)
	return ActiveState{
		pollFreq:    IdleSamplingFrequency,
		desc:        "Idle",
		shouldSleep: true,
	}
}
