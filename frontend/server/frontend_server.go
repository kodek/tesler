package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"

	"bitbucket.org/kodek64/tesler/common"
	"bitbucket.org/kodek64/tesler/recorder/databases"
	"github.com/golang/glog"
)

// TODO: Turn into a flag
const dbFilename = "tesla.db"

// frontend_server provides a REST interface to Tesler storage.
func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	mux := common.NewKodekMux("Tesler Frontend")

	// TODO: Refactor from recorder_main.go
	defaultHandleFunc := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// Redirect to statusz
		http.Redirect(w, r, "/statusz", http.StatusSeeOther)
	}
	mux.HandleFunc("/", defaultHandleFunc)

	// TODO: Add config loader/handler here if needed.

	// TODO: Refactor from recorder_main.go
	database, err := databases.OpenSqliteDatabase(os.Getenv("HOME") + "/" + dbFilename)
	defer database.Close()
	if err != nil {
		panic(err)
	}

	// TODO: Refactor
	getLatestHandler := func(w http.ResponseWriter, r *http.Request) {
		info, err := database.GetLatest(context.TODO())
		if err != nil {
			http.Error(w, "Error querying database: "+err.Error(), http.StatusInternalServerError)
		}

		js, err := json.Marshal(info)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
	mux.HandleFunc("/latest", getLatestHandler)
	glog.Infof("Starting Tesler Frontend server at %s", ":8081")
	glog.Fatal(http.ListenAndServe(":8081", mux))
}
