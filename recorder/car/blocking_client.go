package car

import (
	"errors"
	"fmt"
	"strings"

	"bitbucket.org/kodek64/tesler/common"
	"github.com/golang/glog"
	"github.com/kodek/tesla"
)

type BlockingClient interface {
	GetUpdate() (Snapshot, error)
}

type teslaBlockingClient struct {
	tc            *tesla.Client
	vin           string
	cachedVehicle *tesla.Vehicle // Access via getVehicle()
}

// returns a BlockingClient for a Tesla vehicle.
func NewTeslaBlockingClient(conf common.Configuration) (BlockingClient, error) {
	tc, err := tesla.NewClient(getTeslaAuth(conf))
	if err != nil {
		return nil, err
	}
	return &teslaBlockingClient{
		tc:  tc,
		vin: conf.Recorder.CarVin,
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

func (c *teslaBlockingClient) GetUpdate() (Snapshot, error) {
	vehicle, err := c.getVehicle()
	if err != nil {
		return Snapshot{}, err
	}

	chargeState, err := vehicle.ChargeState()
	if err != nil {
		return Snapshot{}, err
	}

	streamEvent, err := c.getSingleStreamEvent()
	if err != nil {
		return Snapshot{}, err
	}

	return newSnapshot(vehicle, chargeState, streamEvent), nil
}

// Memoizes the tesla.Vehicle lookup on success.
func (c *teslaBlockingClient) getVehicle() (*tesla.Vehicle, error) {
	if c.cachedVehicle != nil {
		return c.cachedVehicle, nil
	}

	vehicles, err := c.tc.Vehicles()
	if err != nil {
		return nil, err
	}

	for i := range vehicles {
		var vehicle *tesla.Vehicle = vehicles[i].Vehicle
		if strings.ToLower(vehicle.Vin) == strings.ToLower(c.vin) {
			glog.Infof("Found car with VIN %s.", c.vin)
			c.cachedVehicle = vehicle
			return c.cachedVehicle, nil
		}
	}
	return nil, errors.New(fmt.Sprintf("No car found with vin %s in Tesla account!", c.vin))
}

func (c *teslaBlockingClient) getSingleStreamEvent() (*tesla.StreamEvent, error) {
	v, err := c.getVehicle()
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
		//eventJSON, _ := json.Marshal(event)
		//fmt.Println(string(eventJSON))
		return event, nil
	case err = <-errChan:
		fmt.Println(err)
		if err.Error() == "HTTP stream closed" {
			fmt.Println("Reconnecting!")
			eventChan, doneChan, errChan, err = v.Stream()
			if err != nil {
				return nil, err
			}
			defer close(doneChan)
		}
	}
	panic("Should not happen")
}
