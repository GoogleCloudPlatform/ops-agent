FROM google/cloud-sdk:latest

RUN rm -rf /usr/local/go

RUN curl -s https://dl.google.com/go/go1.20.linux-amd64.tar.gz | \
  tar --directory /usr/local -xzf /dev/stdin

ENV PATH $PATH:/usr/local/go/bin

RUN go install -v github.com/jstemmer/go-junit-report/v2@latest
