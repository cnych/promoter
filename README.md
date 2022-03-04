# promoter 🍋 🍊 🍒 🍰 🍇 🍉 🍓 🌽

Promoter 是一个用于 AlertManager 报警通知的 Webhooks 实现，目前支持`钉钉`和`企业微信`，支持在消息通知中展示实时报警图表。

![](https://bxdc-static.oss-cn-beijing.aliyuncs.com/images/20220226181006.png)


## 安装

该项目使用 Go 语言编写，有多种方式来安装 Promoter。

### 编译二进制

直接 clone 项目手动构建：
```shell
$ git clone https://github.com/cnych/promoter.git
$ cd promoter
$ go build -a -o promoter cmd/promoter/main.go
$ ./promoter --config.file=<your_file>
```

### Docker 镜像

Promoter 镜像上传到了 Docker Hub，你可以尝试使用下面的命令来启动服务：

```shell
$ docker run --name promoter -d -p 8080:8080 cnych/promoter:v0.1.2
```

## 配置

下面的配置基本上覆盖了 Promoter 使用的配置：

```yaml
global:
  prometheus_url: http://192.168.31.31:30104
  wechat_api_secret: <secret>
  wechat_api_corp_id: <secret>
  dingtalk_api_token: <secret>
  dingtalk_api_secret: <secret>

s3:
  access_key: <secret>
  secret_key: <secret>
  endpoint: oss-cn-beijing.aliyuncs.com
  region: cn-beijing
  bucket: <bucket>

receivers:
  - name: rcv1
    wechat_config:
      agent_id: <agent_id>
      to_user: "@all"
      message_type: markdown
      message: '{{ template "wechat.default.message" . }}'
    dingtalk_config:
      message_type: markdown
      markdown:
        title: '{{ template "dingtalk.default.title" . }}',
        text: '{{ template "dingtalk.default.content" . }}',
        at:
          atMobiles: [ "123456" ]
          isAtAll: false
```

在 global 下面可以配置全局属性，比如企业微信或者钉钉的密钥，S3 下面是一个对象存储（阿里云 OSS 也可以）配置，用来保存监控图标生成的图片。

`receivers` 下面是配置的各种消息的接收器，可以在一个接收器中同时配置企业微信和钉钉，支持 `text` 和 `markdown` 两种格式，其中的 `name` 非常中，
比如这里名称叫`rcv1`，那么该接收器的 Webhook 地址为：`http://<promoter-url>/rcv1/send`，在 AlertManager Webhook 中需要配置该地址。

> 需要注意企业微信的 Markdown 格式不支持直接展示图片

## 模板

默认模板位于 `template/default.tmpl`，可以根据自己需求定制：

```tmpl
{{ define "__subject" }}[{{ .Status | toUpper }}{{ if eq .Status "firing" }}:{{ .Alerts.Firing | len }}{{ end }}] {{ .GroupLabels.SortedPairs.Values | join " " }} {{ if gt (len .CommonLabels) (len .GroupLabels) }}({{ with .CommonLabels.Remove .GroupLabels.Names }}{{ .Values | join " " }}{{ end }}){{ end }}{{ end }}

{{ define "default.__text_alert_list" }}{{ range . }}
**{{ .Annotations.summary }}**

{{ range .Images }}
![click there get alert image]({{ .Url }})
{{- end }}

**description:**
> {{ .Annotations.description }}

**labels:**
{{ range .Labels.SortedPairs }}{{ if and (ne (.Name) "severity") (ne (.Name) "summary") }}> - {{ .Name }}: {{ .Value | markdown | html }}
{{ end }}{{ end }}
{{ end }}{{ end }}

{{ define "dingtalk.default.title" }}{{ template "__subject" . }}{{ end }}
{{ define "dingtalk.default.content" }}
{{ if gt (len .Alerts.Firing) 0 -}}
### {{ .Alerts.Firing | len }} Alerts Firing:
{{ template "default.__text_alert_list" .Alerts.Firing }}
{{ range .AtMobiles }}@{{ . }}{{ end }}
{{- end }}
{{ if gt (len .Alerts.Resolved) 0 -}}
### **{{ .Alerts.Resolved | len }} Alerts Resolved:**
{{ template "default.__text_alert_list" .Alerts.Resolved }}
{{ range .AtMobiles }}@{{ . }}{{ end }}
{{- end }}
{{- end }}

{{ define "wechat.default.message" }}
{{ if gt (len .Alerts.Firing) 0 -}}
### {{ .Alerts.Firing | len }} Alerts Firing:
> {{ template "default.__text_alert_list" .Alerts.Firing }}
{{- end }}
{{ if gt (len .Alerts.Resolved) 0 -}}
### **{{ .Alerts.Resolved | len }} Alerts Resolved:**
{{ template "default.__text_alert_list" .Alerts.Resolved }}
{{- end }}
{{- end }}
{{ define "wechat.default.to_user" }}{{ end }}
{{ define "wechat.default.to_party" }}{{ end }}
{{ define "wechat.default.to_tag" }}{{ end }}
{{ define "wechat.default.agent_id" }}{{ end }}
```
