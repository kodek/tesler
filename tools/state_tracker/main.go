// Checks if the car is online.
package main

import (
	"flag"
	"time"

	"github.com/golang/glog"
	influxdb "github.com/influxdata/influxdb/client/v2"
	"github.com/kodek/tesla"
	"github.com/kodek/tesler/common"
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	glog.Info("Loading config")
	conf := common.LoadConfig()

	// Open database
	influxClient := initDatabase(conf)
	defer influxClient.Close()

	// Open Tesla API
	teslaClient := initTesla(conf)

	for {
		Sample(conf, influxClient, teslaClient)
		time.Sleep(1 * time.Minute)
	}

}

func Sample(conf common.Configuration, influxClient influxdb.Client, teslaClient *tesla.Client) {
	glog.Info("Starting sample")
	vehicles, err := teslaClient.Vehicles()
	if err != nil {
		glog.Errorf("Error while getting vehicles: %s", err)
		return
	}

	for i := range vehicles {
		var vehicle = vehicles[i].Vehicle
		if vehicle == nil {
			glog.Errorf("Vehicle at index %d is null! This is unexpected.", i)
			return
		}
		glog.Infof("Recording vehicle: %s", vehicle.Vin)
		recordVehicle(conf, influxClient, vehicle)
	}
	glog.Info("Done!")
}

func recordVehicle(conf common.Configuration, influxClient influxdb.Client, vehicle *tesla.Vehicle) {
	bp, err := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database:  conf.Recorder.InfluxDbConfig.Database,
		Precision: "s",
	})
	if err != nil {
		panic(err)
	}

	tags := map[string]string{
		"car_name": vehicle.DisplayName,
		"vin":      vehicle.Vin,
		"binary":   "tools/state_tracker",
	}

	var carState string
	if vehicle.State == nil {
		carState = "null"
	} else {
		carState = *vehicle.State
	}

	state, err := influxdb.NewPoint(
		"online_state",
		tags,
		map[string]interface{}{
			"state": carState,
		},
		time.Now())
	if err != nil {
		panic(err)
	}

	bp.AddPoint(state)

	err = influxClient.Write(bp)
	if err != nil {
		panic(err)
	}

}

func initTesla(conf common.Configuration) *tesla.Client {
	teslaConf := conf.Recorder.TeslaAuth
	auth := &tesla.Auth{
		ClientID:     teslaConf.ClientId,
		ClientSecret: teslaConf.ClientSecret,
		Email:        teslaConf.Username,
		Password:     teslaConf.Password,
	}
	tc, err := tesla.NewClient(auth)
	if err != nil {
		panic(err)
	}
	return tc
}

func initDatabase(conf common.Configuration) influxdb.Client {
	influxConf := conf.Recorder.InfluxDbConfig
	// TODO: Check that config isn't empty/missing.
	// Create a new HTTPClient
	c, err := influxdb.NewHTTPClient(influxdb.HTTPConfig{
		Addr:     influxConf.Address,
		Username: influxConf.Username,
		Password: influxConf.Password,
	})
	if err != nil {
		panic(err)
	}
	return c
}
