# Build the manager binary
FROM golang:1.17 as builder

RUN apt-get -y update && apt-get -y install upx

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the go source
COPY . .

# Build
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
ENV GO111MODULE=on
ENV GOPROXY="https://goproxy.cn"

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download && \
    go build -a -o promoter main.go && \
    upx manager promoter

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot as app
WORKDIR /
COPY --from=builder /workspace/promoter /
COPY --from=builder /workspace/config.example.yaml  /etc/promoter/config.yaml
COPY --from=builder /workspace/template/default.tmpl /templates/default.tmpl
USER nonroot:nonroot

ENTRYPOINT ["/promoter"]

