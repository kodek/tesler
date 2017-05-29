package car

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/kodek/tesla"
)

type BlockingClient interface {
	GetUpdate() (Snapshot, error)
}

type ChargeSession struct {
	Voltage          float64
	ActualCurrent    float64
	PilotCurrent     float64
	TimeToFullCharge float64
	ChargeMilesAdded float64
	ChargeRate       float64
}

type Bearings struct {
	Latitude  float64
	Longitude float64
	Speed     int
}

type Snapshot struct {
	Timestamp      time.Time
	Name           string
	DrivingState   string
	Bearings       Bearings
	ChargingState  string
	BatteryLevel   int
	RangeLeft      float64
	ChargeLimitSoc int
	ChargeSession  *ChargeSession
	Odometer       float64
}

type teslaBlockingClient struct {
	tc            *tesla.Client
	vin           string
	cachedVehicle *tesla.Vehicle // Access via GetVehicle()
}

func NewTeslaBlockingClient(vin string, teslaClient *tesla.Client) (BlockingClient, error) {
	return &teslaBlockingClient{
		tc:  teslaClient,
		vin: vin,
	}, nil
}

func (c *teslaBlockingClient) GetUpdate() (Snapshot, error) {
	vehicle, err := c.getVehicle()
	if err != nil {
		return nil, err
	}

	chargeState, err := vehicle.ChargeState()
	if err != nil {
		return nil, err
	}

	streamEvent, err := c.getSingleStreamEvent()
	if err != nil {
		return nil, err
	}

	return newSnapshot(vehicle, chargeState, streamEvent), nil
}

func newSnapshot(vehicleResponse *tesla.Vehicle, chargeStateResponse *tesla.ChargeState, streamEventResponse *tesla.StreamEvent) Snapshot {
	var chargeSession *ChargeSession
	if chargeStateResponse.ChargingState != "Disconnected" {
		chargeSession = &toChargeSession(chargeStateResponse)
	}
	return Snapshot{
		Timestamp:    time.Now(),
		Name:         vehicleResponse.DisplayName,
		Odometer:     streamEventResponse.Odometer,
		DrivingState: streamEventResponse.ShiftState,
		Bearings: Bearings{
			Latitude:  streamEventResponse.EstLat,
			Longitude: streamEventResponse.EstLng,
			Speed:     streamEventResponse.Speed,
		},
		ChargingState:  chargeStateResponse.ChargingState,
		BatteryLevel:   chargeStateResponse.BatteryLevel,
		RangeLeft:      chargeStateResponse.BatteryRange,
		ChargeLimitSoc: chargeStateResponse.ChargeLimitSoc,
		ChargeSession:  chargeSession,
	}
}

func toChargeSession(response *tesla.ChargeState) ChargeSession {
	session := ChargeSession{
		// TODO: If this fails, go back to a TimeToFullCharge pointer and &response.TimeToFulCharge.
		TimeToFullCharge: response.TimeToFullCharge,
		ChargeMilesAdded: response.ChargeMilesAddedRated,
		ChargeRate:       response.ChargeRate,
	}

	// Potentially missing fields.
	if val, ok := response.ChargerVoltage.(float64); ok {
		session.Voltage = val
	}
	if val, ok := response.ChargerActualCurrent.(float64); ok {
		session.ActualCurrent = val
	}
	if val, ok := response.ChargerPilotCurrent.(float64); ok {
		session.PilotCurrent = val
	}
	return session
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

	for _, vehicle := range vehicles {
		if strings.ToLower(vehicle.Vin) == strings.ToLower(c.vin) {
			glog.Infof("Found car with VIN %s.", c.vin)
			c.cachedVehicle = &tesla.Vehicle(vehicle)
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
