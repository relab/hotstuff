proto_include := $(shell go list -m -f {{.Dir}} github.com/relab/gorums)
proto_src := internal/proto/clientpb/clientpb.proto internal/proto/hotstuffpb/hotstuff.proto
proto_go := $(proto_src:%.proto=%.pb.go)
gorums_go := $(proto_src:%.proto=%_gorums.pb.go)

binaries := cmd/hotstuff/hotstuff

.PHONY: all debug clean protos download tools $(binaries)

all: $(binaries)

debug: GCFLAGS += -gcflags='all=-N -l'
debug: $(binaries)

$(binaries): protos
	@go build -o ./$@ $(GCFLAGS) ./$(dir $@)

protos: $(proto_go) $(gorums_go)

download:
	@go mod download

tools: download
	@cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -I % go install %

clean:
	@rm -fv $(binaries)

%.pb.go %_gorums.pb.go : %.proto
	protoc -I=$(proto_include):. \
		--go_out=paths=source_relative:. \
		--gorums_out=paths=source_relative:. \
		$<
