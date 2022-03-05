package receiver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/cnych/promoter/config"
	"github.com/cnych/promoter/notify"
	"github.com/cnych/promoter/notify/dingtalk"
	"github.com/cnych/promoter/notify/wechat"
	"github.com/cnych/promoter/template"
	"github.com/cnych/promoter/util"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/route"
)

type API struct {
	mtx sync.RWMutex

	config            *config.Config
	tmpl              *template.Template
	receiverNotifiers map[string][]ReceiveNotifier
	logger            log.Logger
	debug             bool
}

type ReceiveNotifier struct {
	receiver *config.Receiver
	notifier notify.Notifier
}

func New(logger log.Logger, debug bool) *API {
	return &API{
		logger: logger,
		debug:  debug,
	}
}

func (api *API) Register(r *route.Router) {
	r.Post("/:name/send", api.serveReceiver)
}

func (api *API) serveReceiver(w http.ResponseWriter, r *http.Request) {
	receiverName := route.Param(r.Context(), "name")
	logger := log.With(api.logger, "receiver", receiverName)

	receiverNotifiers := api.receiverNotifiers[receiverName]
	if len(receiverNotifiers) == 0 {
		level.Warn(logger).Log("msg", "receiver not found")
		http.NotFound(w, r)
		return
	}

	var data notify.Data
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		level.Error(logger).Log("msg", "Cannot decode prometheus webhook JSON request", "err", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// 生成监控图片
	if err := data.MakeAlertImages(logger, api.config); err != nil {
		level.Error(logger).Log("msg", "Cannot make alert images", "err", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	errs := &util.MultiError{}

	for _, rn := range receiverNotifiers {
		if _, err := rn.notifier.Notify(context.Background(), &data); err != nil {
			errs.Add(err)
		}
	}

	if errs.Len() > 0 {
		level.Error(logger).Log("msg", "Send receiver notify failed", "err", errs)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	io.WriteString(w, "OK")
}

func (api *API) Update(conf *config.Config, tmpl *template.Template) {
	api.mtx.Lock()
	defer api.mtx.Unlock()

	api.config = conf
	api.tmpl = tmpl

	// 将 Receivers 映射成 map，获取每个接收器的 notifier
	var receiverNotifier = make(map[string][]ReceiveNotifier)
	for _, rcv := range api.config.Receivers {
		var receiverNotifiers []ReceiveNotifier
		if rcv.DingtalkConfig != nil {
			notifier, err := dingtalk.New(rcv.DingtalkConfig, api.tmpl, api.logger)
			if err != nil {
				level.Error(api.logger).Log("msg", "Init dingtalk notifier", "err", err)
				continue
			}
			receiverNotifiers = append(receiverNotifiers, ReceiveNotifier{
				receiver: rcv,
				notifier: notifier,
			})
		}
		if rcv.WechatConfig != nil {
			notifier, err := wechat.New(rcv.WechatConfig, api.tmpl, api.logger)
			if err != nil {
				level.Error(api.logger).Log("msg", "Init wechat notifier", "err", err)
				continue
			}
			receiverNotifiers = append(receiverNotifiers, ReceiveNotifier{
				receiver: rcv,
				notifier: notifier,
			})
		}
		receiverNotifier[rcv.Name] = receiverNotifiers
	}
	api.receiverNotifiers = receiverNotifier
}
