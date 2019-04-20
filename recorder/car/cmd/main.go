package main

import (
	"flag"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/gregdel/pushover"
	"github.com/kodek/tesla"
	"github.com/kodek/tesler/common"
	"github.com/kodek/tesler/recorder/car"
)

func main() {
	flag.Set("logtostderr", "true")
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

	countAdapter := func(in func(v *tesla.Vehicle)) func(v *tesla.Vehicle) {
		count := 0
		return func(v *tesla.Vehicle) {
			count = count + 1
			in(v)
			glog.Infof("Count for %s is %d.", v.DisplayName, count)
		}
	}

	ignoreFirstAdapter := func(in func(v *tesla.Vehicle)) func(v *tesla.Vehicle) {
		isFirst := true
		return func(v *tesla.Vehicle) {
			if isFirst {
				glog.Infof("Ignored %s processing because it is first.", v.DisplayName)
				isFirst = false

				message := pushover.NewMessage(fmt.Sprintf("Monitoring for %s is ready!", v.DisplayName))
				_, err := push.SendMessage(message, pushUser)
				if err != nil {
					glog.Errorf("Cannot send Pushover message: ", err)
				}
				return
			}

			in(v)
		}
	}

	logAndNotifyListener := func(v *tesla.Vehicle) {
		glog.Info("Vehicle %s state changed: %s", v.DisplayName, spew.Sdump(v))
		message := pushover.NewMessageWithTitle(
			spew.Sdump(v),
			fmt.Sprintf("Vehicle %s state changed to %+v", v.DisplayName, v.State))
		_, err := push.SendMessage(message, pushUser)

		if err != nil {
			glog.Errorf("Cannot send Pushover message: ", err)
		}
	}

	for _, c := range conf.Recorder.Cars {
		if !c.Monitor {
			glog.Infof("Skipping VIN %s. Monitoring disabled in config.", c.Vin)
			continue
		}
		poller.AddListenerFunc(c.Vin, countAdapter(ignoreFirstAdapter(logAndNotifyListener)))
	}
	poller.Start()
}
