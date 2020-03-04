FROM golang:alpine

ADD . /go/src/github.com/percona/usermanager

WORKDIR /go/src/github.com/percona/usermanager

RUN go install

CMD ["usermanager"]