package recorder

import (
	"fmt"
	"time"

	"bitbucket.org/kodek64/tesler/common"

	"context"

	"github.com/cenkalti/backoff"
	"github.com/golang/glog"
	"github.com/jsgoecke/tesla"
)

const refreshDuration = 15 * time.Second

func Start(ctx context.Context, conf common.Configuration) <-chan CarInfo {

	out := make(chan CarInfo)

	c, err := tesla.NewClient(getTeslaAuth(conf))
	if err != nil {
		panic(err)
	}

	var info *CarInfo = nil
	refreshInfo := func() error {
		i, err := getCarInfo(c)
		if err != nil {
			return err
		}
		info = i
		return nil
	}

	onError := func(e error, d time.Duration) {
		glog.Errorf("Error. Retrying in (%s): %s\n", common.Round(d, time.Millisecond), e)
	}

	retryStrategy := backoff.NewExponentialBackOff()
	retryStrategy.MaxElapsedTime = 0
	// Loop forever
	go func() {
		for {
			backoff.RetryNotify(refreshInfo, retryStrategy, onError)
			select {
			case out <- *info:
				glog.Info("Updated CarInfo")
			case <-ctx.Done():
				glog.Info("Tesla canceled")
			}
			time.Sleep(refreshDuration)
		}
	}()
	return out
}

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
