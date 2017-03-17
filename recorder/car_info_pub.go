package recorder

import (
	"context"
	"time"

	"bitbucket.org/kodek64/tesler/common"
	"github.com/cenkalti/backoff"
	"github.com/golang/glog"
	"github.com/jsgoecke/tesla"
)

const refreshDuration = 15 * time.Second

type teslaPubHelper struct {
	client *tesla.Client
	out    chan CarInfo
}

// NewCarInfoPublisher returns a channel that provides CarInfo updates.
// TODO: Consider only publishing event changes.
func NewCarInfoPublisher(ctx context.Context, conf common.Configuration) (<-chan CarInfo, error) {
	c, err := tesla.NewClient(getTeslaAuth(conf))
	if err != nil {
		return nil, err
	}

	out := make(chan CarInfo)

	t := &teslaPubHelper{
		client: c,
		out:    out,
	}

	go t.updateIndefinitely(ctx)

	return out, nil
}

func (t *teslaPubHelper) updateIndefinitely(ctx context.Context) {
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
		case <-ctx.Done():
			glog.Info("Tesla canceled")
			return
		default:
		}
		time.Sleep(refreshDuration)
	}

}
