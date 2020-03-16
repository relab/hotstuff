PROTOPKG  := ./proto
PROTOBUF  := $(PROTOPKG)/hotstuff.pb.go
GORUMS    := $(PROTOPKG)/hotstuff_gorums.pb.go 
GRPC      := $(PROTOPKG)/hotstuff_grpc.pb.go
PROTOFILE := $(PROTOPKG)/hotstuff.proto

.PHONY: all
all: $(PROTOBUF) $(GORUMS)

$(PROTOBUF) $(GORUMS) &: $(PROTOFILE)
	protoc -I=$(GOPATH)/src:. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=paths=source_relative:. \
		$(PROTOFILE)
