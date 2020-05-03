package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/golang/glog"
	"github.com/kodek/tesla"
	"github.com/kodek/tesler/common"
	"github.com/kodek/tesler/recorder/car"
	"github.com/kodek/tesler/recorder/databases"
	"github.com/pkg/errors"
)

// Recorder dumps data from the given Vehicle into a Database while the vehicle is actively being used.
type Recorder struct {
	recording bool
	Database  databases.Database
}

func NewRecorder(d databases.Database) (*Recorder, error) {
	return &Recorder{
		Database: d,
	}, nil
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

// activeState represents the state of the vehicle, as it's actively being polled.
type activeState struct {
	shouldSleep bool
	pollFreq    time.Duration
	desc        string
}

func (s *activeState) PollInterval() time.Duration {
	return s.pollFreq
}

func (s *activeState) Description() string {
	return s.desc
}

func (s *activeState) ShouldSleep() bool {
	return s.shouldSleep
}

func newActiveState(data *tesla.VehicleData) activeState {
	shiftState := data.DriveState.ShiftState

	if data.DriveState.Speed > 0 {
		glog.Infof("Car %s is actively moving.", data.Vin)
		return activeState{
			pollFreq: 1 * time.Second,
			desc:     "Moving",
		}
	}
	if shiftState == "R" || shiftState == "D" || shiftState == "N" {
		glog.Infof("Car %s not moving, but in gear.", data.Vin)
		return activeState{
			pollFreq: 2 * time.Second,
			desc:     "In gear",
		}
	}

	chargeState := data.ChargeState.ChargingState
	if chargeState == "Charging" || chargeState == "Starting" {
		glog.Infof("Car %s has active charge state: %s.", data.Vin, chargeState)
		return activeState{
			pollFreq: 3 * time.Second,
			desc:     "Charging",
		}
	}

	if data.VehicleState.SentryMode {
		glog.Infof("Sentry mode enabled for %s.", data.Vin)
		return activeState{
			pollFreq: 30 * time.Second,
			desc:     "Sentry mode",
		}
	}

	if data.VehicleState.CenterDisplayState != 0 {
		glog.Infof("Center display is on for car %s.", data.Vin)
		return activeState{
			pollFreq: 10 * time.Second,
			desc:     "Display on",
		}
	}

	if data.ClimateState.IsClimateOn {
		glog.Infof("Climate is on for car %s.", data.Vin)
		return activeState{
			pollFreq: 30 * time.Second,
			desc:     "Climate on",
		}
	}

	glog.Infof("Car %s is not active.", data.Vin)
	return activeState{
		pollFreq:    IdleSamplingFrequency,
		desc:        "Idle",
		shouldSleep: true,
	}
}
