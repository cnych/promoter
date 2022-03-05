package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
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
	tmpl          *template.Template
	conf          *config.WechatConfig
	client        *http.Client
	logger        log.Logger
	accessToken   string
	accessTokenAt time.Time
}

type token struct {
	AccessToken string `json:"access_token"`
}

// New 返回一个新的 Wechat notifier 对象
func New(c *config.WechatConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (notify.Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "wechat", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{conf: c, tmpl: t, logger: l, client: client}, nil
}

func (n *Notifier) Notify(ctx context.Context, data *notify.Data) (bool, error) {
	var err error
	tmpl := notify.TmplText(n.tmpl, data, &err)
	if err != nil {
		return false, err
	}

	// 超过2小时刷新 AccessToken
	if n.accessToken == "" || time.Since(n.accessTokenAt) > 2*time.Hour {
		parameters := url.Values{}
		parameters.Add("corpsecret", string(n.conf.APISecret))
		parameters.Add("corpid", string(n.conf.CorpID))

		u := n.conf.APIURL.Copy()
		u.Path += "gettoken"
		u.RawQuery = parameters.Encode()

		resp, err := util.Get(ctx, n.client, u.String())
		if err != nil {
			return true, util.RedactURL(err)
		}
		defer util.Drain(resp)

		var wechatToken token
		if err := json.NewDecoder(resp.Body).Decode(&wechatToken); err != nil {
			return false, err
		}

		if wechatToken.AccessToken == "" {
			return false, fmt.Errorf("invalid APISecret for CorpID: %s", n.conf.CorpID)
		}

		// 缓存 token
		n.accessToken = wechatToken.AccessToken
		n.accessTokenAt = time.Now()
	}

	msg := &weChatMessage{
		ToUser:  tmpl(n.conf.ToUser),
		ToParty: tmpl(n.conf.ToParty),
		Totag:   tmpl(n.conf.ToTag),
		AgentID: tmpl(n.conf.AgentID),
		Type:    n.conf.MessageType,
		Safe:    "0",
	}
	if msg.Type == "markdown" {
		msg.Markdown = weChatMessageContent{
			Content: tmpl(n.conf.Message),
		}
	} else if msg.Type == "template_card" {
		msg.TemplateCard = weChatMessageTemplateCard{
			CardType: "news_notice",
			MainTitle: weChatMessageTemplateMainTitle{
				Title: tmpl(n.conf.TemplateCard.Title),
				Desc:  tmpl(n.conf.TemplateCard.Description),
			},
			ImageTextArea: wechatMessageTemplateImage{
				Type:     1,
				URL:      n.tmpl.ExternalURL.String(),
				Title:    tmpl(n.conf.TemplateCard.Title),
				Desc:     tmpl(n.conf.TemplateCard.Description),
				ImageURL: tmpl(n.conf.TemplateCard.ImageURL),
			},
		}
	} else {
		msg.Text = weChatMessageContent{
			Content: tmpl(n.conf.Message),
		}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	postMessageURL := n.conf.APIURL.Copy()
	postMessageURL.Path += "message/send"
	q := postMessageURL.Query()
	q.Set("access_token", n.accessToken)
	postMessageURL.RawQuery = q.Encode()

	resp, err := util.PostJSON(ctx, n.client, postMessageURL.String(), &buf)
	if err != nil {
		return false, util.RedactURL(err)
	}
	defer util.Drain(resp)

	if resp.StatusCode != 200 {
		return true, fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	level.Debug(n.logger).Log("response", string(body))
	if err != nil {
		return false, err
	}

	var weResp weChatResponse
	if err := json.Unmarshal(body, &weResp); err != nil {
		return false, err
	}

	// https://work.weixin.qq.com/api/doc#10649
	if weResp.Code == 0 {
		return false, nil
	}

	// AccessToken is expired
	if weResp.Code == 42001 {
		n.accessToken = ""
		return true, errors.New(weResp.Error)
	}

	return false, errors.New(weResp.Error)
}

type weChatMessage struct {
	Text         weChatMessageContent      `json:"text,omitempty"`
	ToUser       string                    `json:"touser,omitempty"`
	ToParty      string                    `json:"toparty,omitempty"`
	Totag        string                    `json:"totag,omitempty"`
	AgentID      string                    `json:"agentid,omitempty"`
	Safe         string                    `json:"safe,omitempty"`
	Type         string                    `json:"msgtype,omitempty"`
	Markdown     weChatMessageContent      `json:"markdown,omitempty"`
	TemplateCard weChatMessageTemplateCard `json:"template_card,omitempty"`
	News         weChatMessageNews         `json:"news,omitempty"`
}

type weChatMessageContent struct {
	Content string `json:"content"`
}

type weChatMessageNews struct {
}

type weChatMessageTemplateCard struct {
	CardType      string                         `json:"card_type"`
	MainTitle     weChatMessageTemplateMainTitle `json:"main_title"`
	ImageTextArea wechatMessageTemplateImage     `json:"image_text_area"`
}

type weChatMessageTemplateMainTitle struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

type wechatMessageTemplateImage struct {
	Type     int    `json:"type"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	ImageURL string `json:"image_url"`
}

type weChatResponse struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}
