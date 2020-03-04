PROTOPKG  := ./pkg/proto
PROTOBUF  := $(PROTOPKG)/hotstuff.pb.go
GORUMS    := $(PROTOPKG)/hotstuff.gorums.go 
PROTOFILE := $(PROTOPKG)/hotstuff.proto

.PHONY: all
all: $(PROTOBUF) $(GORUMS)

$(PROTOBUF) $(GORUMS) &: $(PROTOFILE)
	protoc -I=${GOPATH}/src:. --gorums_out=plugins=grpc+gorums:$(PROTOPKG) $(PROTOFILE)
