INTERNALPATH   := ./internal/proto
INTERNALPROTO  := $(INTERNALPATH)/hotstuff.pb.go
INTERNALGORUMS := $(INTERNALPATH)/hotstuff_gorums.pb.go
INTERNALFILE   := $(INTERNALPATH)/hotstuff.proto

PUBLICPATH   := ./clientapi
PUBLICPROTO  := $(PUBLICPATH)/hotstuff.pb.go
PUBLICGORUMS := $(PUBLICPATH)/hotstuff_gorums.pb.go
PUBLICFILE   := $(PUBLICPATH)/hotstuff.proto

CLIENTPATH := ./cmd/hotstuffclient
CLIENT     := $(CLIENTPATH)/hotstuffclient

SERVERPATH := ./cmd/hotstuffserver
SERVER     := $(SERVERPATH)/hotstuffserver

.PHONY: all $(CLIENT) $(SERVER)

all: $(CLIENT) $(SERVER)

$(CLIENT): $(PUBLICPROTO) $(PUBLICGORUMS)
	@go build -o $(CLIENT) $(CLIENTPATH)

$(SERVER): $(PUBLICPROTO) $(PUBLICGORUMS) $(INTERNALPROTO) $(INTERNALGORUMS)
	@go build -o $(SERVER) $(SERVERPATH)

$(INTERNALPROTO): $(INTERNALFILE)
	@protoc -I=$(GOPATH)/src:. \
		--go_out=paths=source_relative:. \
		$(INTERNALFILE)

$(INTERNALGORUMS): $(INTERNALFILE)
	@protoc -I=$(GOPATH)/src:. \
		--gorums_out=paths=source_relative:. \
		$(INTERNALFILE)

$(PUBLICPROTO): $(PUBLICFILE)
	@protoc -I=$(GOPATH)/src:. \
		--go_out=paths=source_relative:. \
		$(PUBLICFILE)

$(PUBLICGORUMS): $(PUBLICFILE)
	@protoc -I=$(GOPATH)/src:. \
		--gorums_out=paths=source_relative:. \
		$(PUBLICFILE)
