package recorder

import (
	"time"

	"bitbucket.org/kodek64/tesler/common"
	"github.com/cenkalti/backoff"
	"github.com/golang/glog"
	"github.com/kodek/tesla"
)

// TODO: Should be a flag
const drivingRefreshDuration = 15 * time.Second
const normalRefreshDuration = 5 * time.Minute
const chargingRefreshDuration = 1 * time.Minute
const sleepingRefreshDuration = 15 * time.Minute

type teslaPubHelper struct {
	client *tesla.Client
	out    chan CarInfo
}

// NewCarInfoPublisher returns a channel that provides CarInfo updates.
// TODO: Consider only publishing event changes.
func NewCarInfoPublisher(conf common.Configuration) (<-chan CarInfo, chan<- bool, error) {
	c, err := tesla.NewClient(getTeslaAuth(conf))
	if err != nil {
		return nil, nil, err
	}

	out := make(chan CarInfo)

	t := &teslaPubHelper{
		client: c,
		out:    out,
	}

	stop := make(chan bool)
	go t.updateIndefinitely(stop)

	return out, stop, nil
}

func (t *teslaPubHelper) updateIndefinitely(stop <-chan bool) {
	var state *CarInfo = nil
	doRefreshFn := func() error {
		i, err := getCarInfo(t.client)
		if err != nil {
			return err
		}
		state = i
		return nil
	}

	onError := func(e error, d time.Duration) {
		glog.Errorf("Error. Retrying in (%s): %s\n", common.Round(d, time.Millisecond), e)
	}

	retryStrategy := backoff.NewExponentialBackOff()
	retryStrategy.MaxElapsedTime = 0
	defer close(t.out)

	limiter := newRateLimiter()
	for {
		// TODO: Allow cancelling of retry via context cancellation channel.
		backoff.RetryNotify(doRefreshFn, retryStrategy, onError)
		glog.Info("Updated CarInfo")
		select {
		case t.out <- *state: // Send update to client.
			break
		case <-stop: // Don't send further updates.
			glog.Info("Tesla sampling stopped.")
			return
		default: // The client wasn't ready for the update.
		}
		limiter.RateLimit(state)
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

func (rl *rateLimiter) RateLimit(carState *CarInfo) {
	// Rate limit
	<-rl.ticker.C
	rl.ticker.Stop()

	// Select a new ticker based on current state

	// Fast ticking: car is being used.
	if carState.DrivingState != "" {
		rl.ticker = time.NewTicker(drivingRefreshDuration)
		glog.Infof("Fast refreshing due to use: %s", drivingRefreshDuration)
		return
	}
	// Normal ticking: car is charging
	if carState.Charge != nil {
		rl.ticker = time.NewTicker(chargingRefreshDuration)
		glog.Infof("Refreshing due to charging: %s", chargingRefreshDuration)
		return
	}

	// It's between midnight and 8 am and car isn't charging or being used.
	// TODO: Change to car is not charging and it hasn't been used in 2 hours.
	now := time.Now()
	if now.Hour() >= 0 && now.Hour() <= 8 {
		rl.ticker = time.NewTicker(sleepingRefreshDuration)
		glog.Infof("Slow refreshing due to time of day (12 to 8 am): %s", sleepingRefreshDuration)
		return

	}
	rl.ticker = time.NewTicker(normalRefreshDuration)
	glog.Infof("Normal refreshing: %s", normalRefreshDuration)
}
