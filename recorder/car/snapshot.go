package car

import (
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/kodek/tesla"
)

type Snapshot struct {
	Timestamp      time.Time
	Name           string
	Vin            string
	WakeState      string // Whether the car is online or not before the remaining REST calls are performed.
	DrivingState   string
	Bearings       Bearings
	ChargingState  string
	BatteryLevel   int
	RangeLeft      float64
	ChargeLimitSoc int
	ChargeSession  *ChargeSession
	Odometer       float64
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
	Speed     float64
}

func NewSnapshot(vehicleData *tesla.VehicleData) *Snapshot {
	glog.Infof("Parsing message: %s", spew.Sdump(vehicleData))
	snapshot := Snapshot{
		Timestamp:      time.Now(),
		Name:           vehicleData.DisplayName,
		Vin:            vehicleData.Vin,
		WakeState:      vehicleData.State,
		ChargingState:  vehicleData.ChargeState.ChargingState,
		BatteryLevel:   vehicleData.ChargeState.BatteryLevel,
		RangeLeft:      vehicleData.ChargeState.BatteryRange,
		ChargeLimitSoc: vehicleData.ChargeState.ChargeLimitSoc,
		ChargeSession:  toChargeSession(vehicleData),
		Odometer:       vehicleData.VehicleState.Odometer,
		Bearings: Bearings{
			Latitude:  vehicleData.DriveState.Latitude,
			Longitude: vehicleData.DriveState.Longitude,
			Speed:     vehicleData.DriveState.Speed,
		},
		DrivingState: vehicleData.DriveState.ShiftState,
	}
	return &snapshot
}

func toChargeSession(parentResponse *tesla.VehicleData) *ChargeSession {
	chargeState := parentResponse.ChargeState
	if chargeState.ChargingState == "Disconnected" || chargeState.ChargingState == "" {
		return nil
	}
	session := &ChargeSession{
		// TODO: If this fails, go back to a TimeToFullCharge pointer and &response.TimeToFulCharge.
		TimeToFullCharge: chargeState.TimeToFullCharge,
		ChargeMilesAdded: chargeState.ChargeMilesAddedRated,
		ChargeRate:       chargeState.ChargeRate,
		Voltage:          chargeState.ChargerVoltage,
		ActualCurrent:    chargeState.ChargerActualCurrent,
		PilotCurrent:     chargeState.ChargerPilotCurrent,
	}

	return session
}
