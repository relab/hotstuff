proto_include := $(shell go list -m -f {{.Dir}} github.com/relab/gorums)
proto_src := clientapi/hotstuff.proto internal/proto/hotstuff.proto
proto_go := $(proto_src:%.proto=%.pb.go)
gorums_go := $(proto_src:%.proto=%_gorums.pb.go)

binaries := cmd/hotstuffclient/hotstuffclient cmd/hotstuffserver/hotstuffserver cmd/hotstuffkeygen/hotstuffkeygen

.PHONY: all debug $(binaries)

all: $(binaries)

debug: GCFLAGS += -gcflags='all=-N -l'
debug: $(binaries)

$(proto_go): %.pb.go : %.proto
	protoc --go_out=paths=source_relative:. -I=$(proto_include):. $^

$(gorums_go): %_gorums.pb.go : %.proto
	protoc --gorums_out=paths=source_relative:. -I=$(proto_include):. $^

$(binaries): $(proto_go) $(gorums_go)
	@go build -o ./$@ $(GCFLAGS) ./$(dir $@)
