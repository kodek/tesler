package recorder

import (
	"time"

	"bitbucket.org/kodek64/tesler/common"
	"github.com/cenkalti/backoff"
	"github.com/golang/glog"
	"github.com/jsgoecke/tesla"
)

// TODO: Should be a flag
const refreshDuration = 1 * time.Minute

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
	refreshInfo := func() error {
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
	for {
		// TODO: Allow cancelling of retry via context cancellation channel.
		backoff.RetryNotify(refreshInfo, retryStrategy, onError)
		glog.Info("Updated CarInfo")
		select {
		case t.out <- *state:
			break
		case <-stop:
			glog.Info("Tesla sampling stopped.")
			return
		default:
		}
		time.Sleep(refreshDuration)
	}

}
