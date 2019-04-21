package car

import (
	"errors"
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/kodek/tesla"
	"github.com/kodek/tesler/common"
)

type BlockingClient interface {
	GetUpdate(vin string) (*Snapshot, error)
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

func (c *teslaBlockingClient) GetUpdate(vin string) (*Snapshot, error) {
	vehicle, err := c.getVehicle(vin)
	if err != nil {
		return nil, err
	}

	chargeState, err := vehicle.ChargeState()
	if err != nil {
		return nil, err
	}

	// TODO: StreamEventResponse is hardcoded to nil. Fetch the entire car's vehicle data in a single API request.
	return newSnapshot(vehicle, chargeState, nil), nil
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
