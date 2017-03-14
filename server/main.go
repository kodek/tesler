package main

import (
	_ "expvar"
	"flag"
	"net/http"

	"github.com/golang/glog"
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	mux := NewKodekMux("Tesler")

	def := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// Redirect to statusz
		http.Redirect(w, r, "/statusz", http.StatusSeeOther)
	}
	mux.HandleFunc("/", def)

	glog.Infof("Starting Tesler server at %s", ":8080")
	glog.Fatal(http.ListenAndServe(":8080", mux))
}
