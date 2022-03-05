ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="cnych <www.qikqiak.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/promoter /bin/promoter
COPY config.example.yaml      /etc/promoter/config.yaml
COPY template/default.tmpl template/default.tmpl

RUN chown -R nobody:nobody etc/promoter

USER       nobody
WORKDIR    /promoter
ENTRYPOINT [ "/bin/promoter" ]
CMD        [ "--config.file=/etc/promoter/config.yaml" ]
