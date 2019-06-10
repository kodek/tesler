//TODO: Move to its own kodekmux package
package common

import (
	"expvar"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"

	"github.com/golang/glog"
)

const (
	statuszTemplateFile = "templates/statusz.html"
)

var (
	httpCounts    = expvar.NewMap("http_counts")
	httpLatencyMs = expvar.NewMap("http_latency_ms")
)

// KodekMux adds middleware functionality to all attached handlers, plus special handlers
// for monitoring (/statusz, /healthz, and expvar metrics)
type KodekMux struct {
	http.ServeMux
	name            string
	patterns        []string // used for statusz reporting
	statuszTemplate *template.Template
}

// NewKodekMux creates a new KodekMux to handle all http requests.
func NewKodekMux(name string) *KodekMux {
	mux := &KodekMux{
		name:            name,
		statuszTemplate: template.Must(template.ParseFiles(statuszTemplateFile)),
	}
	// Don't add middleware to the following
	mux.HandleFunc("/statusz", mux.handleStatusz)
	mux.HandleFunc("/healthz", mux.handleHealthz)
	mux.HandleFunc("/debug/vars", http.DefaultServeMux.ServeHTTP)
	return mux
}

// HandleFunc adds a new Handler function with all middleware.
func (mux *KodekMux) HandleFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	mux.ServeMux.HandleFunc(pattern, wrapHandler(pattern, handler))
	mux.patterns = append(mux.patterns, pattern)
	sort.Strings(mux.patterns)
}

// handleStatusz implements the /statusz handler.
func (mux *KodekMux) handleStatusz(w http.ResponseWriter, r *http.Request) {
	data := struct {
		ServerName          string
		BuildTime           string
		TravisCommit        string
		TravisCommitMessage string
		TravisBuildWebUrl   string
		Patterns            []string
	}{
		ServerName:          mux.name,
		BuildTime:           BuildTime,
		TravisCommit:        TravisCommit,
		TravisCommitMessage: TravisCommitMessage,
		TravisBuildWebUrl:   TravisBuildWebUrl,
		Patterns:            mux.patterns,
	}
	mux.statuszTemplate.Execute(w, data)
}

// handleHealthz implements the /healthz handler.
func (mux *KodekMux) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// TODO: Allow hooks from dependencies to report health.
	fmt.Fprintf(w, "ok")
}

// userHandler represents an external handler.
type userHandler struct {
	name string
}

// wrapHandler wraps a given handler into a userHandler that's then used to attach all middleware.
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

// codeCapturingResponseWriter wraps ResponseWriter to capture the response HTTP code.
type codeCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// newCodeCapturingResponseWriter creates a new codeCapturingResponseWriter.
func newCodeCapturingResponseWriter(w http.ResponseWriter) *codeCapturingResponseWriter {
	return &codeCapturingResponseWriter{w, http.StatusOK}
}

// WriterHeader wraps ResponseWriter and stores the given response code into codeCapturingResponseWriter.
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
