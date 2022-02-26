// Copyright 2015 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"fmt"
	"log"
	"net/url"
	"sort"
	"time"

	"github.com/cnych/promoter/storage"
	"github.com/prometheus/common/model"
	"github.com/spf13/viper"
)

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
	Status string `json:"status"`
	Alerts Alerts `json:"alerts"`

	GroupLabels       KV `json:"groupLabels"`
	CommonLabels      KV `json:"commonLabels"`
	CommonAnnotations KV `json:"commonAnnotations"`

	ExternalURL string `json:"externalURL"`
	AtMobiles   []string
}

// Alert holds one alert for notification templates.
type Alert struct {
	Status      string    `json:"status"`
	Labels      KV        `json:"labels"`
	Annotations KV        `json:"annotations"`
	StartsAt    time.Time `json:"startsAt"`
	EndsAt       time.Time `json:"endsAt"`
	GeneratorURL string    `json:"generatorURL"`
	Fingerprint  string    `json:"fingerprint"`
	Images []DingImage
}

func (a Alert) GetPlotTimeRange() (time.Time, time.Duration) {
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
	log.Printf("Querying Time %v Duration: %v", queryTime, duration)
	return queryTime, duration
}

func (a Alert) GeneratePicture() ([]DingImage, error) {
	generatorUrl, err := url.Parse(a.GeneratorURL)
	if err != nil {
		return nil, err
	}

	generatorQuery, err := url.ParseQuery(generatorUrl.RawQuery)
	if err != nil {
		return nil, err
	}

	var alertFormula string
	for key, param := range generatorQuery {
		if key == "g0.expr" {
			alertFormula = param[0]
			break
		}
	}

	//(node_memory_MemTotal_bytes - (node_memory_MemFree_bytes + node_memory_Buffers_bytes + node_memory_Cached_bytes)) / node_memory_MemTotal_bytes * 100 > 30
	//((1 - sum(increase(node_cpu_seconds_total{mode="idle"}[1m])) by (instance) / sum(increase(node_cpu_seconds_total[1m])) by (instance) ) * 100) > 5

	plotExpression := GetPlotExpr(alertFormula)
	queryTime, duration := a.GetPlotTimeRange()

	var images []DingImage

	for _, expr := range plotExpression {
		plot, err := Plot(
			expr,
			queryTime,
			duration,
			time.Duration(viper.GetInt64("metric_resolution")),
			viper.GetString("prometheus_url"),
			a,
			)
		if err != nil {
			return nil, fmt.Errorf("Plot error: %v\n", err)
		}

		publicURL, err := storage.UploadFile(
			viper.GetString("s3.access_key"),
			viper.GetString("s3.secret_key"),
			viper.GetString("s3.endpoint"),
			viper.GetString("s3.bucket"),
			viper.GetString("s3.region"),
			plot)
		if err != nil {
			return nil, fmt.Errorf("S3 error: %v\n", err)
		}
		log.Printf("Graph uploaded, URL: %s", publicURL)
		images = append(images, DingImage{
			Url:   publicURL,
			Title: expr.String(),
		})
	}

	return images, nil
}

// Alerts is a list of Alert objects.
type Alerts []Alert

// Firing returns the subset of alerts that are firing.
func (as Alerts) Firing() []Alert {
	var res []Alert
	for _, a := range as {
		if a.Status == string(model.AlertFiring) {
			images, err := a.GeneratePicture()
			if err == nil {
				a.Images = append(a.Images, images...)
			}
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
			images, err := a.GeneratePicture()
			if err == nil {
				a.Images = append(a.Images, images...)
			}
			res = append(res, a)
		}
	}
	return res
}
