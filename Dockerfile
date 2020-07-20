FROM ubuntu:20.04

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update
RUN apt-get install -y golang-1.14-go curl unzip
RUN ln -s /usr/lib/go-1.14/bin/go /usr/bin/go
RUN curl -sL https://api.github.com/repos/protocolbuffers/protobuf/releases/latest \
	| grep "browser_download_url" \
	| grep "linux-x86_64" \
	| cut -d '"' -f4 \
	| xargs curl -sL -o protobuf.zip
RUN unzip protobuf.zip -d /usr 

ENV GOBIN /usr/bin
COPY . /src
WORKDIR /src
RUN go mod download
RUN go install ./...
