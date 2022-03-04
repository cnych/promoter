package notify

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/cnych/promoter/config"
	"github.com/cnych/promoter/template"
	"github.com/cnych/promoter/util"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"
)

type Notifier interface {
	Notify(ctx context.Context, alerts *Data) (bool, error)
}

// Pair is a key/value string pair.
type Pair struct {
	Name, Value string
}

// Pairs is a list of key/value string pairs.
type Pairs []Pair

// Names returns a list of names of the pairs.
func (ps Pairs) Names() []string {
	ns := make([]string, 0, len(ps))
	for _, p := range ps {
		ns = append(ns, p.Name)
	}
	return ns
}

// Values returns a list of values of the pairs.
func (ps Pairs) Values() []string {
	vs := make([]string, 0, len(ps))
	for _, p := range ps {
		vs = append(vs, p.Value)
	}
	return vs
}

// KV is a set of key/value string pairs.
type KV map[string]string

// SortedPairs returns a sorted list of key/value pairs.
func (kv KV) SortedPairs() Pairs {
	var (
		pairs     = make([]Pair, 0, len(kv))
		keys      = make([]string, 0, len(kv))
		sortStart = 0
	)
	for k := range kv {
		if k == string(model.AlertNameLabel) {
			keys = append([]string{k}, keys...)
			sortStart = 1
		} else {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys[sortStart:])

	for _, k := range keys {
		pairs = append(pairs, Pair{k, kv[k]})
	}
	return pairs
}

// Remove returns a copy of the key/value set without the given keys.
func (kv KV) Remove(keys []string) KV {
	keySet := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		keySet[k] = struct{}{}
	}

	res := KV{}
	for k, v := range kv {
		if _, ok := keySet[k]; !ok {
			res[k] = v
		}
	}
	return res
}

// Names returns the names of the label names in the LabelSet.
func (kv KV) Names() []string {
	return kv.SortedPairs().Names()
}

// Values returns a list of the values in the LabelSet.
func (kv KV) Values() []string {
	return kv.SortedPairs().Values()
}

// Data is the data passed to notification templates and webhook pushes.
//
// End-users should not be exposed to Go's type system, as this will confuse them and prevent
// simple things like simple equality checks to fail. Map everything to float64/string.
type Data struct {
	Receiver string `json:"receiver"`
	Status   string `json:"status"`
	Alerts   Alerts `json:"alerts"`

	GroupLabels       KV `json:"groupLabels"`
	CommonLabels      KV `json:"commonLabels"`
	CommonAnnotations KV `json:"commonAnnotations"`

	ExternalURL string `json:"externalURL"`
}

func (d *Data) MakeAlertImages(logger log.Logger, config *config.Config) error {
	for i := range d.Alerts {
		generatorUrl, err := url.Parse(d.Alerts[i].GeneratorURL)
		if err != nil {
			return err
		}

		generatorQuery, err := url.ParseQuery(generatorUrl.RawQuery)
		if err != nil {
			return err
		}

		var alertFormula string
		for key, param := range generatorQuery {
			if key == "g0.expr" {
				alertFormula = param[0]
				break
			}
		}

		plotExpression := GetPlotExpr(logger, alertFormula)
		queryTime, duration := d.Alerts[i].getPlotTimeRange()

		for _, expr := range plotExpression {
			plot, err := Plot(
				logger,
				expr,
				queryTime,
				duration,
				time.Duration(config.Global.MetricResolution),
				config.Global.PrometheusURL.String(),
				d.Alerts[i],
			)
			if err != nil {
				return fmt.Errorf("Plot error: %v\n", err)
			}

			publicURL, err := util.UploadFile(
				string(config.S3.AccessKey),
				string(config.S3.SecretKey),
				config.S3.Endpoint,
				config.S3.Bucket,
				config.S3.Region,
				plot)
			if err != nil {
				return fmt.Errorf("S3 error: %v\n", err)
			}

			level.Debug(logger).Log("msg", "alert image uploaded", "url", publicURL)
			d.Alerts[i].Images = append(d.Alerts[i].Images, AlertImage{
				Url:   publicURL,
				Title: expr.String(),
			})

		}

	}

	return nil
}

// Alert holds one alert for notification templates.
type Alert struct {
	Status       string    `json:"status"`
	Labels       KV        `json:"labels"`
	Annotations  KV        `json:"annotations"`
	StartsAt     time.Time `json:"startsAt"`
	EndsAt       time.Time `json:"endsAt"`
	GeneratorURL string    `json:"generatorURL"`
	Fingerprint  string    `json:"fingerprint"`
	Images       []AlertImage
}

func (a Alert) getPlotTimeRange() (time.Time, time.Duration) {
	var queryTime time.Time
	var duration time.Duration
	if a.StartsAt.Second() > a.EndsAt.Second() {
		queryTime = a.StartsAt
		duration = time.Minute * 20
	} else {
		queryTime = a.EndsAt
		duration = queryTime.Sub(a.StartsAt)
		if duration < time.Minute*20 {
			duration = time.Minute * 20
		}
	}
	return queryTime, duration
}

// Alerts is a list of Alert objects.
type Alerts []Alert

// Firing returns the subset of alerts that are firing.
func (as Alerts) Firing() []Alert {
	var res []Alert
	for _, a := range as {
		if a.Status == string(model.AlertFiring) {
			res = append(res, a)
		}
	}
	return res
}

// Resolved returns the subset of alerts that are resolved.
func (as Alerts) Resolved() []Alert {
	var res []Alert
	for _, a := range as {
		if a.Status == string(model.AlertResolved) {
			res = append(res, a)
		}
	}
	return res
}

// TmplText is using monadic error handling in order to make string templating
// less verbose. Use with care as the final error checking is easily missed.
func TmplText(tmpl *template.Template, data *Data, err *error) func(string) string {
	return func(text string) (s string) {
		if *err != nil {
			return
		}
		s, *err = tmpl.ExecuteTextString(text, data)
		return s
	}
}

// TmplHTML is using monadic error handling in order to make string templating
// less verbose. Use with care as the final error checking is easily missed.
func TmplHTML(tmpl *template.Template, data *Data, err *error) func(string) string {
	return func(name string) (s string) {
		if *err != nil {
			return
		}
		s, *err = tmpl.ExecuteHTMLString(name, data)
		return s
	}
}
