
proto_src := clientapi/hotstuff.proto internal/proto/hotstuff.proto
proto_go := $(proto_src:%.proto=%.pb.go)
gorums_go := $(proto_src:%.proto=%_gorums.pb.go)

binaries := cmd/hotstuffclient/hotstuffclient cmd/hotstuffserver/hotstuffserver cmd/hotstuffkeygen/hotstuffkeygen

.PHONY: all $(binaries)

all: $(binaries)

$(proto_go): %.pb.go : %.proto
	@protoc --go_out=paths=source_relative:. -I=$(GOPATH)/src:. $^

$(gorums_go): %_gorums.pb.go : %.proto
	@protoc --gorums_out=paths=source_relative:. -I=$(GOPATH)/src:. $^

$(binaries): $(proto_go) $(gorums_go)
	@go build -o ./$@ ./$(dir $@)
