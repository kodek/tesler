package recorder

import (
	"fmt"
	"time"

	"encoding/json"

	"bitbucket.org/kodek64/tesler/common"
	"github.com/jsgoecke/tesla"
)

func getTeslaAuth(conf common.Configuration) *tesla.Auth {
	teslaConf := conf.TeslaAuth
	return &tesla.Auth{
		ClientID:     teslaConf.ClientId,
		ClientSecret: teslaConf.ClientSecret,
		Email:        teslaConf.Username,
		Password:     teslaConf.Password,
	}
}

type ChargeInfo struct {
	Voltage          float64
	ActualCurrent    float64
	PilotCurrent     float64
	TimeToFullCharge *float64
}

type CarPosition struct {
	Latitude  float64
	Longitude float64
	Speed     int
}

type CarInfo struct {
	Timestamp     time.Time
	Name          string
	DrivingState  string
	Position      CarPosition
	ChargingState string
	BatteryLevel  int
	Charge        *ChargeInfo
}

func getCarInfo(client *tesla.Client) (*CarInfo, error) {
	// TODO: Support multiple vehicles
	vehicles, err := client.Vehicles()
	if err != nil {
		return nil, err
	}
	// ASSERT 1 car
	if len(vehicles) != 1 {
		panic(fmt.Sprintf("Expected exactly one vehicle: %+v", vehicles))
	}
	vehicle := vehicles[0]

	charge, err := vehicle.ChargeState()
	if err != nil {
		return nil, err
	}

	var cInfo *ChargeInfo = nil
	if val, ok := charge.ChargerVoltage.(float64); ok && val != 0 {
		cInfo = &ChargeInfo{
			Voltage:          charge.ChargerVoltage.(float64),
			ActualCurrent:    charge.ChargerActualCurrent.(float64),
			PilotCurrent:     charge.ChargerPilotCurrent.(float64),
			TimeToFullCharge: &charge.TimeToFullCharge,
		}
	}

	firstStreamEvent, err := getSingleStreamEvent(vehicle.Vehicle)
	if err != nil {
		return nil, err
	}

	info := &CarInfo{
		Timestamp:    time.Now(),
		Name:         vehicle.DisplayName,
		DrivingState: firstStreamEvent.ShiftState,
		Position: CarPosition{
			Latitude:  firstStreamEvent.EstLat,
			Longitude: firstStreamEvent.EstLng,
			Speed:     firstStreamEvent.Speed,
		},
		ChargingState: charge.ChargingState,
		BatteryLevel:  charge.BatteryLevel,
		Charge:        cInfo,
	}
	return info, nil
}

func getSingleStreamEvent(vehicle *tesla.Vehicle) (*tesla.StreamEvent, error) {
	eventChan, errChan, err := vehicle.Stream()
	if err != nil {
		return nil, err
	}
	select {
	case event := <-eventChan:
		eventJSON, _ := json.Marshal(event)
		fmt.Println(string(eventJSON))
		return event, nil
	case err = <-errChan:
		fmt.Println(err)
		if err.Error() == "HTTP stream closed" {
			fmt.Println("Reconnecting!")
			eventChan, errChan, err = vehicle.Stream()
			if err != nil {
				return nil, err
			}
		}
	}

}
