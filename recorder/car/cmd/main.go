package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/gregdel/pushover"
	"github.com/kodek/tesla"
	"github.com/kodek/tesler/common"
	"github.com/kodek/tesler/recorder/car"
	"github.com/kodek/tesler/recorder/databases"
)

func main() {
	_ = flag.Set("logtostderr", "true")
	flag.Parse()

	glog.Info("Loading config")
	conf := common.LoadConfig()

	push := pushover.New(conf.Recorder.Pushover.Token)
	pushUser := pushover.NewRecipient(conf.Recorder.Pushover.User)

	// Open Tesla API
	teslaClient, err := car.NewTeslaClientFromConfig(conf)
	if err != nil {
		panic(err)
	}

	poller, err := car.NewPoller(teslaClient)
	if err != nil {
		panic(err)
	}

	newCountAdapter := func(in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
		// NOTE: Not thread-safe.
		count := 0
		return func(v *tesla.Vehicle) {
			count = count + 1
			glog.Infof("Count for %s is %d.", v.DisplayName, count)
			in(v)
		}
	}

	newIgnoreFirstAdapter := func(in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
		// NOTE: Not thread-safe.
		isFirst := true
		return func(v *tesla.Vehicle) {
			if isFirst {
				glog.Infof("Ignored %s processing because it is first.", v.DisplayName)
				isFirst = false

				message := pushover.NewMessage(fmt.Sprintf("Monitoring for %s is ready!", v.DisplayName))
				_, err := push.SendMessage(message, pushUser)
				if err != nil {
					glog.Errorf("Cannot send Pushover message: %s", err)
				}
				return
			}

			in(v)
		}
	}

	// Open database
	var database databases.Database
	// Uncomment to use sqlite.
	//database, err = databases.OpenSqliteDatabase(os.Getenv("HOME") + "/" + sqliteDb)
	influxConf := conf.Recorder.InfluxDbConfig
	// TODO: Check that config isn't empty/missing.
	database, err = databases.OpenInfluxDbDatabase(
		influxConf.Address,
		influxConf.Username,
		influxConf.Password,
		influxConf.Database)
	if err != nil {
		panic(err)
	}
	defer database.Close()

	recordMetricsAdapter := func(in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
		return func(v *tesla.Vehicle) {
			if v.State == nil || *v.State != "online" {
				glog.Infof("Not recording metrics for %s because it's going offline.", v.DisplayName)
				return
			}
			vd, err := v.VehicleData()
			if err != nil {
				glog.Errorf("Cannot fetch vehicle data for %s: %s", v.DisplayName, err)
				return
			}

			err = database.Insert(context.Background(), *car.NewSnapshot(vd))
			if err != nil {
				glog.Fatalf("Cannot write to database: %s", err)
			}

			in(v)
		}
	}
	logAndNotifyListener := func(v *tesla.Vehicle) {
		glog.Infof("Vehicle %s state changed: %s", v.DisplayName, spew.Sdump(v))
		message := pushover.NewMessageWithTitle(
			spew.Sdump(v),
			fmt.Sprintf("Vehicle %s state changed to %+v", v.DisplayName, v.State))
		_, err := push.SendMessage(message, pushUser)

		if err != nil {
			glog.Errorf("Cannot send Pushover message: %s", err)
		}
	}

	for _, c := range conf.Recorder.Cars {
		onlyRunThisCarFn := func(in car.OnVehicleChangeFunc) car.OnVehicleChangeFunc {
			thisVin := c.Vin
			thisMonitor := c.Monitor
			return func(v *tesla.Vehicle) {
				if v.Vin != thisVin {
					// Skip the car. Hopefully some other handler will match it.
					// TODO: Restructure code so that if a new vin shows up (outside the config), an error is logged.
					return
				}
				if !thisMonitor {
					glog.Infof("Ignored update for VIN %s. Monitoring disabled in config.", v.Vin)
					return
				}
				in(v)
			}
		}
		poller.AddVehicleChangeListener(
			onlyRunThisCarFn(
				newCountAdapter(
					newIgnoreFirstAdapter(
						recordMetricsAdapter(
							logAndNotifyListener)))))
	}
	poller.Start()
}
