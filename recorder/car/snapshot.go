package car

import (
	"time"

	"github.com/kodek/tesla"
)

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

func newSnapshot(
	vehicleResponse *tesla.Vehicle,
	chargeStateResponse *tesla.ChargeState,
	streamEventResponse *tesla.StreamEvent) Snapshot {
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
		ChargeSession:  toChargeSession(chargeStateResponse),
	}
}

func toChargeSession(response *tesla.ChargeState) *ChargeSession {
	if response.ChargingState == "Disconnected" {
		return nil
	}
	session := &ChargeSession{
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