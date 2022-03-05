package api

import (
	"net/http"

	rcvapi "github.com/cnych/promoter/api/receiver"
	apiv1 "github.com/cnych/promoter/api/v1"
	"github.com/cnych/promoter/config"
	"github.com/cnych/promoter/template"
	"github.com/go-kit/log"
	"github.com/prometheus/common/route"
)

type Options struct {
	Logger log.Logger
	Debug  bool
}

type API struct {
	v1       *apiv1.API
	receiver *rcvapi.API
}

func New(opts Options) *API {
	l := opts.Logger
	if l == nil {
		l = log.NewNopLogger()
	}
	v1 := apiv1.New(log.With(l, "component", "apiv1"))
	receiverAPI := rcvapi.New(log.With(l, "component", "receiver"), opts.Debug)

	return &API{
		v1:       v1,
		receiver: receiverAPI,
	}
}

func (api *API) Register(r *route.Router) *http.ServeMux {
	api.v1.Register(r.WithPrefix("/api/v1"))
	api.receiver.Register(r)

	mux := http.NewServeMux()
	mux.Handle("/", r)

	return mux
}

// Update updates the config field of the API struct
func (api *API) Update(conf *config.Config, tmpl *template.Template) {
	api.v1.Update(conf)
	api.receiver.Update(conf, tmpl)
}
