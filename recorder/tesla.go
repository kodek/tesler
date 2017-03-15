package recorder

import (
	"fmt"
	"time"

	"github.com/jsgoecke/tesla"
	"bitbucket.org/kodek64/tesler/common"
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

type CarInfo struct {
	LastUpdate    time.Time
	Name          string
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

	info := &CarInfo{
		LastUpdate:    time.Now(),
		Name:          vehicle.DisplayName,
		ChargingState: charge.ChargingState,
		BatteryLevel:  charge.BatteryLevel,
		Charge:        cInfo,
	}
	return info, nil
}
