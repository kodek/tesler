package recorder

import (
	"time"

	"bitbucket.org/kodek64/tesler/common"
	"bitbucket.org/kodek64/tesler/recorder/car"
	"github.com/cenkalti/backoff"
	"github.com/golang/glog"
)

// TODO: Should be a flag
const drivingRefreshDuration = 15 * time.Second
const normalRefreshDuration = 30 * time.Minute
const chargingRefreshDuration = 1 * time.Minute
const sleepingRefreshDuration = 1 * time.Hour

type teslaPubHelper struct {
	carClient car.BlockingClient
	out       chan car.Snapshot
}

// NewCarInfoPublisher returns a channel that provides CarInfo updates.
// TODO: Consider only publishing event changes.
func NewCarInfoPublisher(conf common.Configuration) (<-chan car.Snapshot, chan<- bool, error) {
	// DO NOT SUBMIT: Move VIN to config.
	carClient, err := car.NewTeslaBlockingClient(conf)
	if err != nil {
		return nil, nil, err
	}

	out := make(chan car.Snapshot)

	t := &teslaPubHelper{
		carClient: carClient,
		out:       out,
	}

	stop := make(chan bool)
	go t.updateIndefinitely(stop)

	return out, stop, nil
}

func (t *teslaPubHelper) updateIndefinitely(stop <-chan bool) {
	var latestSnapshot car.Snapshot
	doRefreshFn := func() error {
		snapshot, err := t.carClient.GetUpdate()
		if err != nil {
			return err
		}
		latestSnapshot = snapshot
		return nil
	}

	onError := func(e error, d time.Duration) {
		glog.Errorf("Error. Retrying in (%s): %s\n", common.Round(d, time.Millisecond), e)
	}

	retryStrategy := backoff.NewExponentialBackOff()
	retryStrategy.MaxElapsedTime = 0
	defer close(t.out)

	limiter := newRateLimiter()
	glog.Infof("Initializing rate limiter. First tick will be fast.")
	for {
		// TODO: Allow cancelling of retry via context cancellation channel.
		backoff.RetryNotify(doRefreshFn, retryStrategy, onError)
		glog.Info("Updated car snapshot")
		select {
		case t.out <- latestSnapshot: // Send update to client.
			break
		case <-stop: // Don't send further updates.
			glog.Info("Car sampling stopped.")
			return
		default: // The client wasn't ready for the update.
		}
		limiter.RateLimit(latestSnapshot)
	}
}

type rateLimiter struct {
	ticker *time.Ticker
}

func newRateLimiter() rateLimiter {
	return rateLimiter{
		// Use a fast rate for the first tick.
		ticker: time.NewTicker(drivingRefreshDuration),
	}
}

func (rl *rateLimiter) RateLimit(latestSnapshot car.Snapshot) {
	// Rate limit
	<-rl.ticker.C
	rl.ticker.Stop()

	// Select a new ticker based on current state

	// Fast ticking: car is being used.
	if latestSnapshot.DrivingState != "" {
		rl.ticker = time.NewTicker(drivingRefreshDuration)
		glog.Infof("Fast refreshing due to use: %s", drivingRefreshDuration)
		return
	}
	// Normal ticking: car is charging
	if latestSnapshot.ChargeSession != nil {
		if latestSnapshot.ChargingState == "Complete" {
			glog.Infof("Plugged in, but fully charged. Not using charging refresh rate.")
		} else {
			glog.Infof("Refreshing due to charging (not fully charged): %s", chargingRefreshDuration)
			rl.ticker = time.NewTicker(chargingRefreshDuration)
			return
		}
	}

	// It's between midnight and 8 am and car isn't charging or being used.
	// TODO: Change to car is not charging and it hasn't been used in 2 hours.
	now := time.Now()
	if now.Hour() >= 0 && now.Hour() <= 8 {
		rl.ticker = time.NewTicker(sleepingRefreshDuration)
		glog.Infof("Slow refreshing due to hour of day %s being between 12 to 8 am: %s", now.Hour(), sleepingRefreshDuration)
		return

	}
	rl.ticker = time.NewTicker(normalRefreshDuration)
	glog.Infof("Normal refreshing, car likely parked/stopped: %s", normalRefreshDuration)
}
