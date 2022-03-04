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

package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

const secretToken = "<secret>"

var secretTokenJSON string

func init() {
	b, err := json.Marshal(secretToken)
	if err != nil {
		panic(err)
	}
	secretTokenJSON = string(b)
}

// Secret is a string that must not be revealed on marshaling.
type Secret string

// MarshalYAML implements the yaml.Marshaler interface for Secret.
func (s Secret) MarshalYAML() (interface{}, error) {
	if s != "" {
		return secretToken, nil
	}
	return nil, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Secret.
func (s *Secret) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Secret
	return unmarshal((*plain)(s))
}

// MarshalJSON implements the json.Marshaler interface for Secret.
func (s Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal(secretToken)
}

// URL is a custom type that represents an HTTP or HTTPS URL and allows validation at configuration load time.
type URL struct {
	*url.URL
}

// Copy makes a deep-copy of the struct.
func (u *URL) Copy() *URL {
	v := *u.URL
	return &URL{&v}
}

// MarshalYAML implements the yaml.Marshaler interface for URL.
func (u URL) MarshalYAML() (interface{}, error) {
	if u.URL != nil {
		return u.URL.String(), nil
	}
	return nil, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for URL.
func (u *URL) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	urlp, err := parseURL(s)
	if err != nil {
		return err
	}
	u.URL = urlp.URL
	return nil
}

// MarshalJSON implements the json.Marshaler interface for URL.
func (u URL) MarshalJSON() ([]byte, error) {
	if u.URL != nil {
		return json.Marshal(u.URL.String())
	}
	return []byte("null"), nil
}

// UnmarshalJSON implements the json.Marshaler interface for URL.
func (u *URL) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	urlp, err := parseURL(s)
	if err != nil {
		return err
	}
	u.URL = urlp.URL
	return nil
}

// SecretURL is a URL that must not be revealed on marshaling.
type SecretURL URL

// MarshalYAML implements the yaml.Marshaler interface for SecretURL.
func (s SecretURL) MarshalYAML() (interface{}, error) {
	if s.URL != nil {
		return secretToken, nil
	}
	return nil, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SecretURL.
func (s *SecretURL) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	// In order to deserialize a previously serialized configuration (eg from
	// the Alertmanager API with amtool), `<secret>` needs to be treated
	// specially, as it isn't a valid URL.
	if str == secretToken {
		s.URL = &url.URL{}
		return nil
	}
	return unmarshal((*URL)(s))
}

// MarshalJSON implements the json.Marshaler interface for SecretURL.
func (s SecretURL) MarshalJSON() ([]byte, error) {
	return json.Marshal(secretToken)
}

// UnmarshalJSON implements the json.Marshaler interface for SecretURL.
func (s *SecretURL) UnmarshalJSON(data []byte) error {
	// In order to deserialize a previously serialized configuration (eg from
	// the Alertmanager API with amtool), `<secret>` needs to be treated
	// specially, as it isn't a valid URL.
	if string(data) == secretToken || string(data) == secretTokenJSON {
		s.URL = &url.URL{}
		return nil
	}
	return json.Unmarshal(data, (*URL)(s))
}

// Load parses the YAML input s into a Config.
func Load(s string) (*Config, error) {
	cfg := &Config{}
	err := yaml.UnmarshalStrict([]byte(s), cfg)
	if err != nil {
		return nil, err
	}

	cfg.original = s
	return cfg, nil
}

// LoadFile parses the given YAML file into a Config.
func LoadFile(filename string) (*Config, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := Load(string(content))
	if err != nil {
		return nil, err
	}

	resolveFilepaths(filepath.Dir(filename), cfg)
	return cfg, nil
}

// resolveFilepaths joins all relative paths in a configuration
// with a given base directory.
func resolveFilepaths(baseDir string, cfg *Config) {
	join := func(fp string) string {
		if len(fp) > 0 && !filepath.IsAbs(fp) {
			fp = filepath.Join(baseDir, fp)
		}
		return fp
	}

	for i, tf := range cfg.Templates {
		cfg.Templates[i] = join(tf)
	}

	cfg.Global.HTTPConfig.SetDirectory(baseDir)
	for _, receiver := range cfg.Receivers {
		if receiver.WechatConfig != nil {
			receiver.WechatConfig.HTTPConfig.SetDirectory(baseDir)
		}
		if receiver.DingtalkConfig != nil {
			receiver.DingtalkConfig.HTTPConfig.SetDirectory(baseDir)
		}
	}
}

// Config 整个应用最顶层的配置文件
type Config struct {
	Global    *GlobalConfig `yaml:"global,omitempty" json:"global,omitempty"`
	Receivers []*Receiver   `yaml:"receivers,omitempty" json:"receivers,omitempty"`
	Templates []string      `yaml:"templates" json:"templates"`
	S3        *S3Config     `yaml:"s3" json:"s3"`
	// original is the input from which the config was parsed.
	original string
}

type S3Config struct {
	AccessKey Secret `yaml:"access_key" json:"access_key"`
	SecretKey Secret `yaml:"secret_key" json:"secret_key"`
	Endpoint  string `yaml:"endpoint" json:"endpoint"`
	Region    string `yaml:"region" json:"region"`
	Bucket    string `yaml:"bucket" json:"bucket"`
}

func (c Config) GetReceiver(name string) *Receiver {
	for _, rcv := range c.Receivers {
		if name == rcv.Name {
			return rcv
		}
	}
	return nil
}

func (c Config) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Config.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// We want to set c to the defaults and then overwrite it with the input.
	// To make unmarshal fill the plain data struct rather than calling UnmarshalYAML
	// again, we have to hide it using a type indirection.
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// 如果 global 配置块放开但是没配置内容，则需要用默认的值覆盖
	if c.Global == nil {
		c.Global = &GlobalConfig{}
		*c.Global = DefaultGlobalConfig()
	}

	names := map[string]struct{}{}

	for _, rcv := range c.Receivers {
		// 接收器配置需要唯一
		if _, ok := names[rcv.Name]; ok {
			return fmt.Errorf("notification config name %q is not unique", rcv.Name)
		}
		// 循环 wechat 配置
		if rcv.WechatConfig != nil {
			if rcv.WechatConfig.HTTPConfig == nil {
				rcv.WechatConfig.HTTPConfig = c.Global.HTTPConfig
			}
			if rcv.WechatConfig.APIURL == nil {
				if c.Global.WeChatAPIURL == nil {
					return fmt.Errorf("no global Wechat URL set")
				}
				rcv.WechatConfig.APIURL = c.Global.WeChatAPIURL
			}
			if rcv.WechatConfig.APISecret == "" {
				if c.Global.WeChatAPISecret == "" {
					return fmt.Errorf("no global Wechat ApiSecret set")
				}
				rcv.WechatConfig.APISecret = c.Global.WeChatAPISecret
			}
			if rcv.WechatConfig.CorpID == "" {
				if c.Global.WeChatAPICorpID == "" {
					return fmt.Errorf("no global Wechat CorpID set")
				}
				rcv.WechatConfig.CorpID = c.Global.WeChatAPICorpID
			}
			if !strings.HasSuffix(rcv.WechatConfig.APIURL.Path, "/") {
				rcv.WechatConfig.APIURL.Path += "/"
			}
		}

		if rcv.DingtalkConfig != nil {
			if rcv.DingtalkConfig.HTTPConfig == nil {
				rcv.DingtalkConfig.HTTPConfig = c.Global.HTTPConfig
			}
			if rcv.DingtalkConfig.APIURL == nil {
				if c.Global.DingTalkAPIURL == nil {
					return fmt.Errorf("no global Dingtalk URL set")
				}
				rcv.DingtalkConfig.APIURL = c.Global.DingTalkAPIURL
			}
			if rcv.DingtalkConfig.APISecret == "" {
				if c.Global.DingTalkAPISecret == "" {
					return fmt.Errorf("no global Dingtalk ApiSecret set")
				}
				rcv.DingtalkConfig.APISecret = c.Global.DingTalkAPISecret
			}
			if rcv.DingtalkConfig.APIToken == "" {
				if c.Global.DingTalkAPIToken == "" {
					return fmt.Errorf("no global Dingtalk ApiToken set")
				}
				rcv.DingtalkConfig.APIToken = c.Global.DingTalkAPIToken
			}
		}

		names[rcv.Name] = struct{}{}
	}

	return nil
}

// DefaultGlobalConfig 返回带默认值的全局配置
func DefaultGlobalConfig() GlobalConfig {
	var defaultHTTPConfig = commoncfg.DefaultHTTPClientConfig
	return GlobalConfig{
		MetricResolution: 100,
		HTTPConfig:       &defaultHTTPConfig,

		WeChatAPIURL:   mustParseURL("https://qyapi.weixin.qq.com/cgi-bin/"),
		DingTalkAPIURL: mustParseURL("https://oapi.dingtalk.com/robot/send"),
	}
}

func mustParseURL(s string) *URL {
	u, err := parseURL(s)
	if err != nil {
		panic(err)
	}
	return u
}

func parseURL(s string) (*URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q for URL", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing host for URL")
	}
	return &URL{u}, nil
}

// HostPort represents a "host:port" network address.
type HostPort struct {
	Host string
	Port string
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for HostPort.
func (hp *HostPort) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var (
		s   string
		err error
	)
	if err = unmarshal(&s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	hp.Host, hp.Port, err = net.SplitHostPort(s)
	if err != nil {
		return err
	}
	if hp.Port == "" {
		return errors.Errorf("address %q: port cannot be empty", s)
	}
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface for HostPort.
func (hp *HostPort) UnmarshalJSON(data []byte) error {
	var (
		s   string
		err error
	)
	if err = json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	hp.Host, hp.Port, err = net.SplitHostPort(s)
	if err != nil {
		return err
	}
	if hp.Port == "" {
		return errors.Errorf("address %q: port cannot be empty", s)
	}
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface for HostPort.
func (hp HostPort) MarshalYAML() (interface{}, error) {
	return hp.String(), nil
}

// MarshalJSON implements the json.Marshaler interface for HostPort.
func (hp HostPort) MarshalJSON() ([]byte, error) {
	return json.Marshal(hp.String())
}

func (hp HostPort) String() string {
	if hp.Host == "" && hp.Port == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", hp.Host, hp.Port)
}

// GlobalConfig 定义全局配置参数
type GlobalConfig struct {
	ExternalURL      *URL  `yaml:"external_url,omitempty" json:"external_url,omitempty"`
	MetricResolution int64 `yaml:"metric_resolution,omitempty" json:"metric_resolution,omitempty"`
	PrometheusURL    *URL  `yaml:"prometheus_url" json:"prometheus_url"` // 配置 prometheus 地址，方便获取监控图表数据

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	WeChatAPIURL    *URL   `yaml:"wechat_api_url,omitempty" json:"wechat_api_url,omitempty"`
	WeChatAPISecret Secret `yaml:"wechat_api_secret,omitempty" json:"wechat_api_secret,omitempty"`
	WeChatAPICorpID Secret `yaml:"wechat_api_corp_id,omitempty" json:"wechat_api_corp_id,omitempty"`

	DingTalkAPIURL    *URL   `yaml:"dingtalk_api_url,omitempty" json:"dingtalk_api_url,omitempty"`
	DingTalkAPIToken  Secret `yaml:"dingtalk_api_token,omitempty" json:"dingtalk_api_token,omitempty"`
	DingTalkAPISecret Secret `yaml:"dingtalk_api_secret,omitempty" json:"dingtalk_api_secret,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for GlobalConfig.
func (c *GlobalConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultGlobalConfig()
	type plain GlobalConfig
	return unmarshal((*plain)(c))
}

// Receiver configuration provides configuration on how to contact a receiver.
type Receiver struct {
	// A unique identifier for this receiver.
	Name string `yaml:"name" json:"name"`

	//EmailConfigs     []*EmailConfig     `yaml:"email_configs,omitempty" json:"email_configs,omitempty"`
	WechatConfig   *WechatConfig   `yaml:"wechat_config,omitempty" json:"wechat_config,omitempty"`
	DingtalkConfig *DingtalkConfig `yaml:"dingtalk_config,omitempty" json:"dingtalk_config,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Receiver.
func (c *Receiver) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Receiver
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Name == "" {
		return fmt.Errorf("missing name in receiver")
	}
	return nil
}

// MatchRegexps represents a map of Regexp.
type MatchRegexps map[string]Regexp

// UnmarshalYAML implements the yaml.Unmarshaler interface for MatchRegexps.
func (m *MatchRegexps) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain MatchRegexps
	if err := unmarshal((*plain)(m)); err != nil {
		return err
	}
	for k, v := range *m {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
		if v.Regexp == nil {
			return fmt.Errorf("invalid regexp value for %q", k)
		}
	}
	return nil
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshalable.
type Regexp struct {
	*regexp.Regexp
	original string
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Regexp.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	regex, err := regexp.Compile("^(?:" + s + ")$")
	if err != nil {
		return err
	}
	re.Regexp = regex
	re.original = s
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface for Regexp.
func (re Regexp) MarshalYAML() (interface{}, error) {
	if re.original != "" {
		return re.original, nil
	}
	return nil, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface for Regexp
func (re *Regexp) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	regex, err := regexp.Compile("^(?:" + s + ")$")
	if err != nil {
		return err
	}
	re.Regexp = regex
	re.original = s
	return nil
}

// MarshalJSON implements the json.Marshaler interface for Regexp.
func (re Regexp) MarshalJSON() ([]byte, error) {
	if re.original != "" {
		return json.Marshal(re.original)
	}
	return []byte("null"), nil
}
