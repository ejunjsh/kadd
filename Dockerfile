FROM golang

MAINTAINER ejunjsh <sjj050121014@163.com>

WORKDIR /root

RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct

RUN go get github.com/ejunjsh/kps/cmd/kps-controller

ENTRYPOINT [ "kps-controller"]