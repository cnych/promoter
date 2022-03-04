package config

import (
	"regexp"

	"github.com/pkg/errors"
	commoncfg "github.com/prometheus/common/config"
)

var (
	// DefaultWechatConfig defines default values for wechat configurations.
	DefaultWechatConfig = WechatConfig{
		Message: `{{ template "wechat.default.message" . }}`,
		ToUser:  `{{ template "wechat.default.to_user" . }}`,
		ToParty: `{{ template "wechat.default.to_party" . }}`,
		ToTag:   `{{ template "wechat.default.to_tag" . }}`,
		AgentID: `{{ template "wechat.default.agent_id" . }}`,
	}
	// DefaultDingtalkConfig ......
	DefaultDingtalkConfig = DingtalkConfig{
		Markdown: &DingtalkMarkdown{
			Title: `{{ template "dingtalk.default.title" . }}`,
			Text:  `{{ template "dingtalk.default.content" . }}`,
		},
	}
)

// WechatConfig configures notifications via Wechat.
type WechatConfig struct {
	//NotifierConfig `yaml:",inline" json:",inline"`
	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APISecret   Secret `yaml:"api_secret,omitempty" json:"api_secret,omitempty"`
	CorpID      Secret `yaml:"corp_id,omitempty" json:"corp_id,omitempty"`
	Message     string `yaml:"message,omitempty" json:"message,omitempty"`
	TemplateCard *WechatTemplateCard `yaml:"template_card,omitempty" json:"template_card,omitempty"`
	APIURL      *URL   `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	ToUser      string `yaml:"to_user,omitempty" json:"to_user,omitempty"`
	ToParty     string `yaml:"to_party,omitempty" json:"to_party,omitempty"`
	ToTag       string `yaml:"to_tag,omitempty" json:"to_tag,omitempty"`
	AgentID     string `yaml:"agent_id,omitempty" json:"agent_id,omitempty"`
	MessageType string `yaml:"message_type,omitempty" json:"message_type,omitempty"`
}

type WechatTemplateCard struct {
	Title string `yaml:"title" json:"title"`
	Description string `yaml:"desc" json:"desc"`
	ImageURL string `yaml:"image_url" json:"image_url"`
}

const wechatValidTypesRe = `^(text|markdown|template_card|news)$`

var wechatTypeMatcher = regexp.MustCompile(wechatValidTypesRe)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WechatConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultWechatConfig
	type plain WechatConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.MessageType == "" {
		c.MessageType = "text"
	}

	if !wechatTypeMatcher.MatchString(c.MessageType) {
		return errors.Errorf("WeChat message type %q does not match valid options %s", c.MessageType, wechatValidTypesRe)
	}

	return nil
}

type DingtalkConfig struct {
	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APISecret Secret `yaml:"api_secret,omitempty" json:"api_secret,omitempty"`
	APIToken  Secret `yaml:"api_token,omitempty" json:"api_token,omitempty"`
	APIURL    *URL   `yaml:"api_url,omitempty" json:"api_url,omitempty"`

	Text        *DingtalkText       `yaml:"text,omitempty" json:"text,omitempty"`
	Markdown    *DingtalkMarkdown   `yaml:"markdown,omitempty" json:"markdown,omitempty"`
	At          *DingtalkAt         `yaml:"at,omitempty" json:"at,omitempty"`
	MessageType string              `yaml:"message_type,omitempty" json:"message_type,omitempty"`
}

const dingtalkValidTypesRe = `^(text|markdown)$`

var dingtalkTypeMatcher = regexp.MustCompile(dingtalkValidTypesRe)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *DingtalkConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultDingtalkConfig
	type plain DingtalkConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.MessageType == "" {
		c.MessageType = "text"
	}

	if !dingtalkTypeMatcher.MatchString(c.MessageType) {
		return errors.Errorf("Dingtalk message type %q does not match valid options %s", c.MessageType, dingtalkValidTypesRe)
	}

	return nil
}

type DingtalkText struct {
	Title   string `yaml:"title" json:"title"`
	Content string `yaml:"content" json:"content"`
}


type DingtalkMarkdown struct {
	Title string `yaml:"title" json:"title"`
	Text  string `yaml:"text" json:"text"`
}

type DingtalkAt struct {
	AtMobiles []string `yaml:"atMobiles" json:"atMobiles,omitempty"`
	IsAtAll   bool     `yaml:"isAtAll" json:"isAtAll,omitempty"`
}

