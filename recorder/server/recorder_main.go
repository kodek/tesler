package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/kodek/tesler/common"
	"github.com/kodek/tesler/recorder"
	"github.com/kodek/tesler/recorder/databases"
)

// TODO: Turn into flags
func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	mux := common.NewKodekMux("Tesler-Recorder")

	defaultHandleFunc := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// Redirect to statusz
		http.Redirect(w, r, "/statusz", http.StatusSeeOther)
	}
	mux.HandleFunc("/", defaultHandleFunc)

	glog.Infof("Loading config")
	conf := common.LoadConfig()
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		conf.WriteRedacted(w)

	})

	// Open database
	var database databases.Database
	// Uncomment to use sqlite.
	//database, err = databases.OpenSqliteDatabase(os.Getenv("HOME") + "/" + sqliteDb)
	influxConf := conf.Recorder.InfluxDbConfig
	// TODO: Check that config isn't empty/missing.
	database, err := databases.OpenInfluxDbDatabase(
		influxConf.Address,
		influxConf.Username,
		influxConf.Password,
		influxConf.Database)
	if err != nil {
		panic(err)
	}
	defer database.Close()

	// Start listening for car updates.
	updates, _, err := recorder.NewCarInfoPublisher(conf)
	if err != nil {
		panic(err)
	}
	go func() {
		// Process updates.
		for i := range updates {
			glog.Infof("Received: %s", spew.Sdump(i))
			err := database.Insert(context.Background(), i)
			if err != nil {
				glog.Error(err)
			}
		}
	}()

	if conf.Recorder.Port == 0 {
		glog.Fatal("Port 0 currently not supported. Please set config.Recorder.Port to continue.")
	}
	listenSpec := fmt.Sprintf(":%d", conf.Recorder.Port)
	glog.Infof("Starting Tesler recorder server at %s", listenSpec)
	glog.Fatal(http.ListenAndServe(listenSpec, mux))
}
