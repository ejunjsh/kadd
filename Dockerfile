FROM golang

MAINTAINER ejunjsh <sjj050121014@163.com>

WORKDIR /root

RUN go get github.com/ejunjsh/kps

ENTRYPOINT [ "kps-controller"]