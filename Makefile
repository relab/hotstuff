proto_include := $(shell go list -m -f {{.Dir}} github.com/relab/gorums):internal/proto
proto_src := internal/proto/clientpb/client.proto          \
		internal/proto/hotstuffpb/hotstuff.proto           \
		internal/proto/orchestrationpb/orchestration.proto \
		internal/proto/kauripb/kauri.proto                 \
		metrics/types/types.proto
proto_go := $(proto_src:%.proto=%.pb.go)
gorums_go := internal/proto/clientpb/client_gorums.pb.go \
		internal/proto/hotstuffpb/hotstuff_gorums.pb.go  \
		internal/proto/kauripb/kauri_gorums.pb.go

binaries := hotstuff plot

CSV ?= wonderproxy.csv

.PHONY: all aws wonderproxy latencies debug clean protos download tools $(binaries)

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

lint:
	@golangci-lint run ./...

clean:
	@rm -fv $(binaries)

latencies:
	@echo "Generating Latency Matrix using $(CSV)"
	@go run cmd/latencygen/main.go -file "$(CSV)"

wonderproxy:
	@$(MAKE) latencies CSV=wonderproxy.csv

aws:
	@$(MAKE) latencies CSV=aws.csv

%.pb.go %_gorums.pb.go : %.proto
	protoc -I=.:$(proto_include) \
		--go_out=paths=source_relative:. \
		--gorums_out=paths=source_relative:. \
		$<
