PROTOPKG  := ./gorumshotstuff/internal/proto
PROTOBUF  := $(PROTOPKG)/hotstuff.pb.go
GORUMS    := $(PROTOPKG)/hotstuff_gorums.pb.go
PROTOFILE := $(PROTOPKG)/hotstuff.proto

.PHONY: all
all: $(PROTOBUF) $(GORUMS)

# In the future, with GNU Make 4.3 and up, the "gorumsproto" rule will not be necessary
# https://stackoverflow.com/a/59877127

$(PROTOBUF) $(GORUMS): gorumsproto

gorumsproto: $(PROTOFILE)
	@protoc -I=$(GOPATH)/src:. \
		--go_out=paths=source_relative:. \
		--gorums_out=paths=source_relative:. \
		$(PROTOFILE)
