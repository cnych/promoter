package models

import (
	"time"
)

var (
	DefaultConfig = Config{
		Timeout: 5 * time.Second,
	}
	DefaultMessage = Message{
		Title: `{{ template "ding.link.title" . }}`,
		Text:  `{{ template "ding.link.content" . }}`,
	}
)

type Config struct {
	Templates []string      `yaml:"templates,omitempty"`
	DefaultMessage *Message `yaml:"default_message,omitempty"`
	Timeout time.Duration   `yaml:"timeout"`
	Dingtalk *DingtalkConf  `yaml:"dingtalk"`
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultConfig
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return nil
}

func (c *Config) GetDefaultMessage() Message {
	if c.DefaultMessage != nil {
		return *c.DefaultMessage
	}
	return DefaultMessage
}

type Message struct {
	Title string `yaml:"title"`
	Text string `yaml:"text"`
}

type DingtalkConf struct {
	URL string `yaml:"url"`
	Secret string            `yaml:"secret,omitempty"`
	Mention *DingtalkMention `yaml:"mention,omitempty"`
	Message *Message         `yaml:"message,omitempty"`
}

type DingtalkMention struct {
	All     bool     `yaml:"all,omitempty"`
	Mobiles []string `yaml:"mobiles,omitempty"`
}
