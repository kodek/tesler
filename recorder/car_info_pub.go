package recorder

import (
	"time"

	"bitbucket.org/kodek64/tesler/common"
	"bitbucket.org/kodek64/tesler/recorder/car"
	"github.com/cenkalti/backoff"
	"github.com/golang/glog"
)

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

	limiter := car.NewRateLimiter()
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
