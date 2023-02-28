FROM golang:1.20.1-bullseye

RUN go install github.com/jstemmer/go-junit-report/v2@latest