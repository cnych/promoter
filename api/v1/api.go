package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/cnych/promoter/config"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
)

var corsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Accept, Authorization, Content-Type, Origin",
	"Access-Control-Allow-Methods":  "GET, POST, DELETE, OPTIONS",
	"Access-Control-Allow-Origin":   "*",
	"Access-Control-Expose-Headers": "Date",
	"Cache-Control":                 "no-cache, no-store, must-revalidate",
}

// Enables cross-site script calls.
func setCORS(w http.ResponseWriter) {
	for h, v := range corsHeaders {
		w.Header().Set(h, v)
	}
}

type API struct {
	logger log.Logger
	config *config.Config

	uptime time.Time
	mtx    sync.RWMutex
}

func New(l log.Logger) *API {
	if l == nil {
		l = log.NewNopLogger()
	}
	return &API{
		logger: l,
		uptime: time.Now(),
	}
}

func (api *API) Register(r *route.Router) {
	wrap := func(f http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			f(w, r)
		}
	}

	r.Options("/*path", wrap(func(w http.ResponseWriter, r *http.Request) {}))

	r.Get("/status", wrap(api.status))
	r.Get("/receivers", wrap(api.receivers))

	//todo，将报警数据保存
	//r.Get("/alerts", wrap(api.listAlerts))
	//r.Post("/alerts", wrap(api.addAlerts))
}

func (api *API) Update(conf *config.Config) {
	api.mtx.Lock()
	defer api.mtx.Unlock()

	api.config = conf
}

func (api *API) receivers(w http.ResponseWriter, req *http.Request) {
	api.mtx.RLock()
	defer api.mtx.RUnlock()

	receivers := make([]string, 0, len(api.config.Receivers))
	for _, r := range api.config.Receivers {
		receivers = append(receivers, r.Name)
	}

	api.respond(w, receivers)
}

func (api *API) status(w http.ResponseWriter, req *http.Request) {
	api.mtx.RLock()

	var status = struct {
		ConfigYAML  string            `json:"configYAML"`
		ConfigJSON  *config.Config    `json:"configJSON"`
		VersionInfo map[string]string `json:"versionInfo"`
		Uptime      time.Time         `json:"uptime"`
	}{
		ConfigYAML: api.config.String(),
		ConfigJSON: api.config,
		VersionInfo: map[string]string{
			"version":   version.Version,
			"revision":  version.Revision,
			"branch":    version.Branch,
			"buildUser": version.BuildUser,
			"buildDate": version.BuildDate,
			"goVersion": version.GoVersion,
		},
		Uptime: api.uptime,
	}

	api.mtx.RUnlock()

	api.respond(w, status)
}

func (api *API) respond(w http.ResponseWriter, data interface{}) {
	statusMessage := statusSuccess
	b, err := json.Marshal(&response{
		Status: statusMessage,
		Data:   data,
	})
	if err != nil {
		level.Error(api.logger).Log("msg", "error marshaling json response", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if n, err := w.Write(b); err != nil {
		level.Error(api.logger).Log("msg", "error writing response", "bytesWritten", n, "err", err)
	}
}

func (api *API) respondError(w http.ResponseWriter, apiErr *apiError, data interface{}) {
	b, err := json.Marshal(&response{
		Status:    statusError,
		ErrorType: apiErr.typ,
		Error:     apiErr.err.Error(),
		Data:      data,
	})

	if err != nil {
		level.Error(api.logger).Log("msg", "error marshaling json response", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var code int
	switch apiErr.typ {
	case errorBadData:
		code = http.StatusBadRequest
	case errorExec:
		code = 422
	case errorCanceled, errorTimeout:
		code = http.StatusServiceUnavailable
	case errorInternal:
		code = http.StatusInternalServerError
	case errorNotFound:
		code = http.StatusNotFound
	default:
		code = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if n, err := w.Write(b); err != nil {
		level.Error(api.logger).Log("msg", "error writing response", "bytesWritten", n, "err", err)
	}
}

type status string

const (
	statusSuccess status = "success"
	statusError   status = "error"
)

type errorType string

const (
	errorTimeout  errorType = "timeout"
	errorCanceled errorType = "canceled"
	errorExec     errorType = "execution"
	errorBadData  errorType = "bad_data"
	errorInternal errorType = "internal"
	errorNotFound errorType = "not_found"
)

type apiError struct {
	typ errorType
	err error
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s", e.typ, e.err)
}

type response struct {
	Status    status      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType errorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
}
