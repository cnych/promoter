package dingtalk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/cnych/promoter/config"
	"github.com/cnych/promoter/notify"
	"github.com/cnych/promoter/template"
	"github.com/cnych/promoter/util"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	commoncfg "github.com/prometheus/common/config"
)

type Notifier struct {
	tmpl   *template.Template
	conf   *config.DingtalkConfig
	client *http.Client
	logger log.Logger
}

func New(conf *config.DingtalkConfig, tmpl *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (notify.Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "dingtalk", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{conf: conf, tmpl: tmpl, logger: l, client: client}, nil
}

func (n *Notifier) Notify(ctx context.Context, data *notify.Data) (bool, error) {
	var err error
	tmpl := notify.TmplText(n.tmpl, data, &err)
	if err != nil {
		return false, err
	}

	var at dingtalkMessageAt
	if n.conf.At != nil {
		at = dingtalkMessageAt{
			AtMobiles: n.conf.At.AtMobiles,
			IsAtAll:   n.conf.At.IsAtAll,
		}
	}
	msg := &dingtalkMessage{
		Type: n.conf.MessageType,
		At:   &at,
	}
	if msg.Type == "markdown" {
		msg.Markdown = &dingtalkMessageMarkdown{
			Title: tmpl(n.conf.Markdown.Title),
			Text:  tmpl(n.conf.Markdown.Text),
		}
	} else {
		if n.conf.Text != nil {
			msg.Text = &dingtalkMessageText{
				Title:   tmpl(n.conf.Text.Title),
				Content: tmpl(n.conf.Text.Content),
			}
		}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	postMessageURL := n.conf.APIURL.Copy()
	q := postMessageURL.Query()

	if n.conf.APISecret != "" {
		// 如果配置了 Secret，则需要签名
		timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
		strToSign := []byte(timestamp + "\n" + string(n.conf.APISecret))

		mac := hmac.New(sha256.New, []byte(n.conf.APISecret))
		mac.Write(strToSign)
		signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

		q.Set("timestamp", timestamp)
		q.Set("sign", signature)
	}

	q.Set("access_token", string(n.conf.APIToken))
	postMessageURL.RawQuery = q.Encode()

	resp, err := util.PostJSON(ctx, n.client, postMessageURL.String(), &buf)
	if err != nil {
		return true, util.RedactURL(err)
	}
	defer util.Drain(resp)

	if resp.StatusCode != 200 {
		return true, fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return true, err
	}
	level.Debug(n.logger).Log("response", string(body))

	var dtResp dingtalkResponse
	if err := json.Unmarshal(body, &dtResp); err != nil {
		return true, err
	}
	if dtResp.Code != 0 {
		return false, errors.New(dtResp.Message)
	}
	return false, nil
}

type dingtalkResponse struct {
	Message string `json:"errmsg"`
	Code    int    `json:"errcode"`
}

type dingtalkMessage struct {
	Type     string                   `json:"msgtype,omitempty"`
	Text     *dingtalkMessageText     `json:"text,omitempty"`
	Markdown *dingtalkMessageMarkdown `json:"markdown,omitempty"`
	At       *dingtalkMessageAt       `json:"at,omitempty"`
}

type dingtalkMessageText struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type dingtalkMessageMarkdown struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type dingtalkMessageAt struct {
	AtMobiles []string `json:"atMobiles,omitempty"`
	IsAtAll   bool     `json:"isAtAll,omitempty"`
}
