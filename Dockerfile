FROM golang:1.12


COPY . /go/src/process-model-repository
WORKDIR /go/src/process-model-repository

ENV GO111MODULE=on

RUN go build

EXPOSE 8080

CMD ./process-model-repository