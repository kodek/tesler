package car

import (
	"errors"
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/kodek/tesla"
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
func NewTeslaBlockingClient(tc *tesla.Client) (BlockingClient, error) {
	return &teslaBlockingClient{
		tc: tc,
	}, nil
}

func (c *teslaBlockingClient) GetUpdate(vin string) (*Snapshot, error) {
	vehicle, err := c.getVehicle(vin)
	if err != nil {
		return nil, err
	}

	_, err = vehicle.Wakeup()
	if err != nil {
		return nil, err
	}

	vehicleData, err := vehicle.VehicleData()
	if err != nil {
		return nil, err
	}

	return NewSnapshot(vehicleData), nil
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
	_ = c.updateVehicleCache()

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
