package recorder

import (
	"time"

	"errors"

	"bitbucket.org/kodek64/tesler/common"
	"bitbucket.org/kodek64/tesler/recorder/car"
	"github.com/cenkalti/backoff"
	"github.com/golang/glog"
)

type teslaPubHelper struct {
	carClient    car.BlockingClient
	vinsToUpdate []string
	limiters     map[string]car.RateLimiter
	out          chan car.Snapshot
}

// NewCarInfoPublisher returns a channel that provides CarInfo updates.
// TODO: Consider only publishing event changes.
func NewCarInfoPublisher(conf common.Configuration) (<-chan car.Snapshot, chan<- bool, error) {
	carClient, err := car.NewTeslaBlockingClient(conf)
	if err != nil {
		return nil, nil, err
	}

	var vins []string
	limiters := make(map[string]car.RateLimiter)
	for _, confCar := range conf.Recorder.Cars {
		if confCar.Monitor {
			glog.Infof("Monitoring car with VIN %s", confCar.Vin)
			vins = append(vins, confCar.Vin)
			limiters[confCar.Vin] = car.NewRateLimiter()
		} else {
			glog.Infof("Car with VIN %s IGNORED!", confCar.Vin)
		}
	}
	if len(vins) == 0 {
		return nil, nil, errors.New("No cars to monitor found")
	}

	out := make(chan car.Snapshot)

	t := &teslaPubHelper{
		carClient:    carClient,
		vinsToUpdate: vins,
		limiters:     limiters,
		out:          out,
	}

	stop := make(chan bool)
	go t.updateIndefinitely(stop)

	return out, stop, nil
}

func (t *teslaPubHelper) updateIndefinitely(stop <-chan bool) {
	var latestSnapshot car.Snapshot
	var vinToUpdate string
	doRefreshFn := func() error {
		snapshot, err := t.carClient.GetUpdate(vinToUpdate)
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

	glog.Infof("Initializing rate limiter. First tick will be fast.")
	for {
		for _, nextVin := range t.vinsToUpdate {
			vinToUpdate = nextVin
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
			limiter := t.limiters[vinToUpdate]
			limiter.RateLimit(latestSnapshot)
		}
	}
}
