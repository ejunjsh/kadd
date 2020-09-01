FROM golang

MAINTAINER ejunjsh <sjj050121014@163.com>

WORKDIR /root

RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct

COPY . /root

WORKDIR cmd/kadd-controller

RUN go install

ENTRYPOINT [ "kadd-controller"]