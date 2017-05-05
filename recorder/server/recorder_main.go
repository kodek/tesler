package main

import (
	"context"
	_ "expvar"
	"flag"
	"net/http"
	"os"

	"bitbucket.org/kodek64/tesler/common"
	"bitbucket.org/kodek64/tesler/recorder"
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
)

// TODO: Turn into a flag
const dbFilename = "tesla.db"

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	mux := common.NewKodekMux("Tesler")

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

	updates, _, err := recorder.NewCarInfoPublisher(conf)
	if err != nil {
		panic(err)
	}
	database, err := recorder.OpenDatabase(os.Getenv("HOME") + "/" + dbFilename)
	defer database.Close()
	if err != nil {
		panic(err)
	}
	go func() {
		for i := range updates {
			glog.Infof("Received: %s", spew.Sdump(i))
			err := database.Insert(context.Background(), &i)
			if err != nil {
				glog.Error(err)
			}
		}
	}()

	// TODO: Make port autoconf and/or a flag.
	glog.Infof("Starting Tesler server at %s", ":8080")
	glog.Fatal(http.ListenAndServe(":8080", mux))
}