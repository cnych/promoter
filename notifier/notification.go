package notifier

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cnych/promoter/models"
	"github.com/cnych/promoter/template"
	"github.com/pkg/errors"
)

type DingNotificationBuilder struct {
	tmpl *template.Template
	dingtalk *models.DingtalkConf
	titleTpl string
	textTpl string
}

func NewDingNotificationBuilder(tmpl *template.Template, conf *models.Config) *DingNotificationBuilder {
	var (
		defaultMessage = conf.GetDefaultMessage()
		titleTpl = defaultMessage.Title
		textTpl = defaultMessage.Text
	)
	if conf.Dingtalk.Message != nil {
		titleTpl = conf.Dingtalk.Message.Title
		textTpl = conf.Dingtalk.Message.Text
	}
	return &DingNotificationBuilder{
		tmpl: tmpl,
		dingtalk: conf.Dingtalk,
		titleTpl: titleTpl,
		textTpl: textTpl,
	}
}

func (d *DingNotificationBuilder) renderTitle(data interface{}) (string, error) {
	return d.tmpl.ExecuteTextString(d.titleTpl, data)
}

func (d *DingNotificationBuilder) renderText(data interface{}) (string, error) {
	return d.tmpl.ExecuteTextString(d.textTpl, data)
}

func (d *DingNotificationBuilder) Build(data *models.Data) (*models.DingTalkNotification, error) {
	// 将模板中定义的 @ mobiles 添加到 data.AtMobiles 中
	if d.dingtalk.Mention != nil {
		data.AtMobiles = append(data.AtMobiles, d.dingtalk.Mention.Mobiles...)
	}

	title, err := d.renderTitle(data)
	if err != nil {
		return nil, err
	}
	content, err := d.renderText(data)
	if err != nil {
		return nil, err
	}

	notification := &models.DingTalkNotification{
		MessageType: "markdown",
		Markdown: &models.DingTalkNotificationMarkdown{
			Title: title,
			Text: content,
		},
	}

	if d.dingtalk.Mention != nil {
		notification.At = &models.DingTalkNotificationAt{
			IsAtAll: d.dingtalk.Mention.All,
			AtMobiles: d.dingtalk.Mention.Mobiles,
		}
	}

	return notification, nil
}

func (d *DingNotificationBuilder) SendNotification(data *models.Data) (*models.DingTalkNotificationResponse, error) {
	dingtalkUrl, err := url.ParseRequestURI(d.dingtalk.URL)
	if err != nil {
		return nil, err
	}

	if d.dingtalk.Secret != "" {
		// Calculate signature when secret is provided
		timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
		stringToSign := []byte(timestamp + "\n" + string(d.dingtalk.Secret))

		mac := hmac.New(sha256.New, []byte(d.dingtalk.Secret))
		mac.Write(stringToSign) // nolint: errcheck
		signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

		qs := dingtalkUrl.Query()
		qs.Set("timestamp", timestamp)
		qs.Set("sign", signature)
		dingtalkUrl.RawQuery = qs.Encode()
	}

	// 构造发送的通知结构
	notification, err := d.Build(data)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(&notification)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", dingtalkUrl.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy:             http.ProxyFromEnvironment,
			DisableKeepAlives: true,
		},
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		return nil, errors.Errorf("unacceptable response code %d", resp.StatusCode)
	}

	var robotResp models.DingTalkNotificationResponse
	enc := json.NewDecoder(resp.Body)
	if err := enc.Decode(&robotResp); err != nil {
		return nil, err
	}

	return &robotResp, nil
}
