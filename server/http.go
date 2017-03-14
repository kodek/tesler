//TODO: Move to its own kodekmux package
package main

import (
	"expvar"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"

	"github.com/golang/glog"
)

var (
	httpCounts    = expvar.NewMap("http_counts")
	httpLatencyMs = expvar.NewMap("http_latency_ms")
)

var statuszTemplate = template.Must(template.ParseFiles("templates/statusz.html"))

func StartServer(name string) {

	// Set up handlers
}

// KodekMux adds middleware functionality to all attached handlers, plus special handlers
// for monitoring (/statusz, /healthz, and expvar metrics)
type KodekMux struct {
	http.ServeMux
	name     string
	patterns []string // used for statusz reporting
}

// NewKodekMux creates a new KodekMux to handle all http requests.
func NewKodekMux(name string) *KodekMux {
	mux := &KodekMux{
		name: name,
	}
	// Don't add middleware to the following
	mux.HandleFunc("/statusz", mux.handleStatusz)
	mux.HandleFunc("/healthz", mux.handleHealthz)
	mux.HandleFunc("/debug/vars", http.DefaultServeMux.ServeHTTP)
	return mux
}

func (mux *KodekMux) HandleFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	mux.ServeMux.HandleFunc(pattern, wrapHandler(pattern, handler))
	mux.patterns = append(mux.patterns, pattern)
	sort.Strings(mux.patterns)
}

func (mux *KodekMux) handleStatusz(w http.ResponseWriter, r *http.Request) {
	data := struct {
		ServerName string
		Patterns   []string
	}{
		ServerName: mux.name,
		Patterns:   mux.patterns,
	}
	statuszTemplate.Execute(w, data)

	//	fmt.Fprintf(w, "<h1>%s</h1>\n", mux.name)
	//	fmt.Fprintf(w, "<p>up</p>")
	// TODO add list of handlers with links in here
	//	for _, pattern := range mux.patterns {
	//		fmt.Fprintf(w, "<p><a href=\"%s\">%s</a></p>", pattern, pattern)
	//	}
}

func (mux *KodekMux) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// TODO: Allow hooks from dependencies to report health.
	fmt.Fprintf(w, "ok")
}

type userHandler struct {
	name string
}

func wrapHandler(name string, f http.HandlerFunc) http.HandlerFunc {
	h := userHandler{
		name: name,
	}
	return h.metrics(h.logging(f))
}

// metrics exports expvar metrics for the handler.
func (h *userHandler) metrics(f http.HandlerFunc) http.HandlerFunc {
	recordTime := func(start time.Time) {
		sinceStart := time.Since(start)
		httpCounts.Add(h.name, 1)
		httpLatencyMs.Add(h.name, sinceStart.Nanoseconds()/1000)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer recordTime(start)
		f(w, r)
	}
}

type codeCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newCodeCapturingResponseWriter(w http.ResponseWriter) *codeCapturingResponseWriter {
	return &codeCapturingResponseWriter{w, http.StatusOK}
}

func (ccrw *codeCapturingResponseWriter) WriteHeader(code int) {
	ccrw.statusCode = code
	ccrw.ResponseWriter.WriteHeader(code)
}

// logging logs the given request using glog.
func (h *userHandler) logging(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ccrw := newCodeCapturingResponseWriter(w)
		f(ccrw, r)
		glog.Infof("[%s] src: %s for %s %s with response %s (%d)",
			h.name, r.RemoteAddr, r.Method, r.URL, http.StatusText(ccrw.statusCode), ccrw.statusCode)
	}
}
