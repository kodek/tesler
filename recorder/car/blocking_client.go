package car

import (
	"errors"
	"fmt"
	"strings"

	"sync"

	"bitbucket.org/kodek64/tesler/common"
	"github.com/golang/glog"
	"github.com/kodek/tesla"
)

type BlockingClient interface {
	GetUpdate(vin string) (Snapshot, error)
}

type teslaBlockingClient struct {
	tc         *tesla.Client
	vehicles   sync.Map
	vehicleMux sync.Mutex
}

// returns a BlockingClient for a Tesla vehicle.
func NewTeslaBlockingClient(conf common.Configuration) (BlockingClient, error) {
	tc, err := tesla.NewClient(getTeslaAuth(conf))
	if err != nil {
		return nil, err
	}

	return &teslaBlockingClient{
		tc: tc,
	}, nil
}

func getTeslaAuth(conf common.Configuration) *tesla.Auth {
	teslaConf := conf.Recorder.TeslaAuth
	return &tesla.Auth{
		ClientID:     teslaConf.ClientId,
		ClientSecret: teslaConf.ClientSecret,
		Email:        teslaConf.Username,
		Password:     teslaConf.Password,
	}
}

func (c *teslaBlockingClient) GetUpdate(vin string) (Snapshot, error) {
	vehicle, err := c.getVehicle(vin)
	if err != nil {
		return Snapshot{}, err
	}

	chargeState, err := vehicle.ChargeState()
	if err != nil {
		return Snapshot{}, err
	}

	streamEvent, err := c.getSingleStreamEvent(vin)
	if err != nil {
		if strings.Contains(err.Error(), "Can't validate password") {
			// Invalidate vehicle cache. it might have a bad vehicle token.
			c.vehicles.Delete(vin)
		}
		return Snapshot{}, err
	}

	return newSnapshot(vehicle, chargeState, streamEvent), nil
}

// Memoizes the tesla.Vehicle lookup on success.
func (c *teslaBlockingClient) getVehicle(vin string) (*tesla.Vehicle, error) {
	// TODO: re-enable caching once we get enough WakeState data.
	c.vehicleMux.Lock()
	defer c.vehicleMux.Unlock()
	//// Check the cache.
	//val, ok := c.vehicles.Load(vin)
	//if ok {
	//	vehicle := val.(*tesla.Vehicle)
	//	return vehicle, nil
	//}

	// It's not there.
	c.updateVehicleCache()

	// Check the cache again.
	val, ok := c.vehicles.Load(vin)
	if ok {
		vehicle := val.(*tesla.Vehicle)
		return vehicle, nil
	}

	// It's still not there, so it must be missing from the account.
	return nil, errors.New(fmt.Sprintf("No car found with vin %s in Tesla account!", vin))
}

func (c *teslaBlockingClient) updateVehicleCache() error {
	// The vehicle vin wasn't found. Let's try to fetch a new list.
	vehicles, err := c.tc.Vehicles()
	if err != nil {
		return err
	}

	for i := range vehicles {
		var v = vehicles[i].Vehicle
		c.vehicles.Store(v.Vin, v)
		glog.Infof("Found car with VIN %s.", v.Vin)
	}
	return nil
}

func (c *teslaBlockingClient) getSingleStreamEvent(vin string) (*tesla.StreamEvent, error) {
	v, err := c.getVehicle(vin)
	if err != nil {
		return nil, err
	}

	eventChan, doneChan, errChan, err := v.Stream()
	if err != nil {
		return nil, err
	}
	defer close(doneChan)
	select {
	case event := <-eventChan:
		return event, nil
	case err = <-errChan:
		if err.Error() == "HTTP stream closed" {
			fmt.Println("Reconnecting!")
			eventChan, doneChan, errChan, err = v.Stream()
			if err != nil {
				return nil, err
			}
			defer close(doneChan)
		}
		glog.Error(err)
		return nil, err
	}
	panic("Should not happen")
}
