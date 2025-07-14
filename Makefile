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
coverage_bin := hscov
GOCOVERDIR := .log/coverage-data

CSV ?= wonderproxy.csv

.PHONY: all coverage test short aws wonderproxy latencies debug clean protos download tools $(binaries)

all: $(binaries)

coverage: $(coverage_bin)

debug: GCFLAGS += -gcflags='all=-N -l'
debug: $(binaries)

$(binaries): protos
	@go build -o ./$@ $(GCFLAGS) ./cmd/$@

$(coverage_bin): protos
	@go build -cover -coverpkg=./... -o ./$@ $(GCFLAGS) ./cmd/hotstuff

coverage: $(coverage_bin)
	@rm -rf $(GOCOVERDIR)
	@mkdir -p $(GOCOVERDIR)
	@echo "Running tests with coverage..."
	@GOCOVERDIR=$(GOCOVERDIR) ./$(coverage_bin) run --batch-size=100 --cue scripts/local_config.cue &> $(GOCOVERDIR)/log.out
	@go tool covdata percent -i=$(GOCOVERDIR)
	@go tool covdata textfmt -i=$(GOCOVERDIR) -o=$(GOCOVERDIR)/cov.txt
	@echo "Execution output saved to $(GOCOVERDIR)/log.out"
	@echo "To view the coverage report, run:"
	@echo "- Browser view:   go tool cover -html=$(GOCOVERDIR)/cov.txt"
	@echo "- Terminal view:  go tool cover -func=$(GOCOVERDIR)/cov.txt"

protos: $(proto_go) $(gorums_go)

download:
	@go mod download

tools: download
	go install tool

test:
	@go test -v ./... > test.log

short:
	@go test -short ./...

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
