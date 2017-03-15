package main

import (
	_ "expvar"
	"flag"
	"net/http"

	"context"

	"bitbucket.org/kodek64/tesler/common"
	"bitbucket.org/kodek64/tesler/recorder"
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	mux := common.NewKodekMux("Tesler")

	def := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// Redirect to statusz
		http.Redirect(w, r, "/statusz", http.StatusSeeOther)
	}
	mux.HandleFunc("/", def)

	glog.Infof("Loading config")
	conf := common.LoadConfig()
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		conf.WriteRedacted(w)

	})

	info := recorder.Start(context.Background(), conf)
	for i := range info {
		glog.Infof("Received: %s", spew.Sdump(i))
	}

	glog.Infof("Starting Tesler server at %s", ":8080")
	glog.Fatal(http.ListenAndServe(":8080", mux))
}
