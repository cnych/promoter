# promoter 🍋 🍊 🍒 🍰 🍇 🍉 🍓 🌽

Promoter 是一个用于 AlertManager 通知的 Webhooks 实现，目前仅支持了`钉钉`，支持在消息通知中展示实时报警图表。

![](https://bxdc-static.oss-cn-beijing.aliyuncs.com/images/20220226181006.png)

目前是将报警数据渲染成图片后上次到 S3 对象存储，所以需要配置一个对象存储（阿里云 OSS 也可以），此外消息通知展示样式支持模板定制，该功能参考自项目 [prometheus-webhook-dingtalk](https://github.dev/timonwong/prometheus-webhook-dingtalk)。

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

## 部署

默认配置文件如下所示，放置在 `/etc/promoter/config.yaml`：

```yaml
debug: true
http_port: 8080
timeout: 5s
prometheus_url: <prometheus_url>  # Prometheus 的地址
metric_resolution: 100

s3:
  access_key: <ak>  
  secret_key: <sk>
  endpoint: oss-cn-beijing.aliyuncs.com
  region: cn-beijing
  bucket: <bucket>

dingtalk:
  url: https://oapi.dingtalk.com/robot/send?access_token=<token>
  secret: <SEC>  # secret for signature
```

可以直接使用 Docker 镜像 `cnych/promoter:v0.1.1` 部署，在 Kubernetes 中部署可以直接参考 `deploy/kubernetes/promoter.yaml`。

启动完成后在 AlertManager 配置中指定 Webhook 地址即可：

```yaml
route:
  group_by: ['alertname', 'cluster']
  group_wait: 30s
  group_interval: 2m
  repeat_interval: 1h
  receiver: webhook

receivers:
- name: 'webhook'
  webhook_configs:
  - url: 'http://promoter.kube-mon.svc.cluster.local:8080/webhook'  # 配置 promoter 的 webhook 接口
    send_resolved: true
```