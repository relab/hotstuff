proto_include := $(shell go list -m -f {{.Dir}} github.com/relab/gorums)
proto_src := internal/proto/clientpb/client.proto          \
		internal/proto/hotstuffpb/hotstuff.proto           \
		internal/proto/orchestrationpb/orchestration.proto \
		metrics/types/types.proto
proto_go := $(proto_src:%.proto=%.pb.go)
gorums_go := internal/proto/clientpb/client_gorums.pb.go \
		internal/proto/hotstuffpb/hotstuff_gorums.pb.go  \

binaries := hotstuff plot

.PHONY: all debug clean protos download tools $(binaries)

all: $(binaries)

debug: GCFLAGS += -gcflags='all=-N -l'
debug: $(binaries)

$(binaries): protos
	@go build -o ./$@ $(GCFLAGS) ./cmd/$@

protos: $(proto_go) $(gorums_go)

download:
	@go mod download

tools: download
	@cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -I % go install %

test:
	@go test -v ./...

clean:
	@rm -fv $(binaries)

%.pb.go %_gorums.pb.go : %.proto
	protoc -I=$(proto_include):. \
		--go_out=paths=source_relative:. \
		--gorums_out=paths=source_relative:. \
		$<
