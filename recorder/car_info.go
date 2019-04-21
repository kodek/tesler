package recorder

import (
	"time"
)

type ChargeInfo struct {
	Voltage          float64
	ActualCurrent    float64
	PilotCurrent     float64
	TimeToFullCharge *float64
	ChargeMilesAdded float64
	ChargeRate       float64
}

type CarPosition struct {
	Latitude  float64
	Longitude float64
	Speed     int
}

type CarInfo struct {
	Timestamp      time.Time
	Name           string
	DrivingState   string
	Position       CarPosition
	ChargingState  string
	BatteryLevel   int
	RangeLeft      float64
	ChargeLimitSoc int
	Charge         *ChargeInfo
	Odometer       float64
}
