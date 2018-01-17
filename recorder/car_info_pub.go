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
	for _, confCar := range conf.Recorder.Cars {
		if confCar.Monitor {
			glog.Infof("Monitoring car with VIN %s", confCar.Vin)
			vins = append(vins, confCar.Vin)
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
		out:          out,
	}

	stop := make(chan bool)

	// Update all vins in parallel.
	for _, vin := range t.vinsToUpdate {
		_ = vin
		_ = t
		// WARNING: THIS DISABLES SAMPLING. Re-enable to get metrics.
		// go t.updateSingleCarIndefinitely(vin, stop)
	}

	return out, stop, nil
}

func (t *teslaPubHelper) updateSingleCarIndefinitely(vin string, stop <-chan bool) {
	var latestSnapshot car.Snapshot
	doRefreshFn := func() error {
		snapshot, err := t.carClient.GetUpdate(vin)
		if err != nil {
			return err
		}
		latestSnapshot = snapshot
		return nil
	}

	onError := func(e error, d time.Duration) {
		glog.Errorf("Error fetching vin %s. Retrying in (%s): %s\n", vin, common.Round(d, time.Millisecond), e)
	}

	retryStrategy := backoff.NewExponentialBackOff()
	retryStrategy.MaxElapsedTime = 0
	defer close(t.out)

	limiter := car.NewRateLimiter()
	glog.Infof("Initializing rate limiter for vin %s. First tick will be fast.", vin)
	for {
		// TODO: Allow cancelling of retry via context cancellation channel.
		backoff.RetryNotify(doRefreshFn, retryStrategy, onError)
		glog.Infof("Updated car snapshot for vin %s", vin)
		select {
		case t.out <- latestSnapshot: // Send update to client.
			break
		case <-stop: // Don't send further updates.
			glog.Infof("Car vin %s sampling stopped.", vin)
			return
		default: // The client wasn't ready for the update.
		}
		limiter.RateLimit(latestSnapshot)
	}
}
