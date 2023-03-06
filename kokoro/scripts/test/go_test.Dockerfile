FROM golang:1.20.1-bullseye

RUN go install -v github.com/jstemmer/go-junit-report/v2@latest