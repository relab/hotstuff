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

.PHONY: all coverage test short aws wonderproxy latencies debug clean protos download tools act help $(binaries)

.DEFAULT_GOAL := help

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ''
	@echo 'Tip: Test logs are saved to .log/ directory. Search for failures with:'
	@echo '  grep -i "FAIL" .log/test.log'
	@echo '  grep -i "FAIL" .log/act.log'

all: $(binaries) ## Build all binaries (hotstuff, plot)

coverage: $(coverage_bin) ## Build and run coverage tests

debug: GCFLAGS += -gcflags='all=-N -l'
debug: $(binaries) ## Build binaries with debug symbols

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

protos: $(proto_go) $(gorums_go) ## Generate protobuf code

download: ## Download Go module dependencies
	@go mod download

tools: download ## Install development tools
	go install tool

test: ## Run all tests with verbose output (logs to .log/test.log)
	@mkdir -p .log
	@echo "Running tests... (output in .log/test.log)"
	@go test -v ./... > .log/test.log 2>&1 && echo "Tests passed! See .log/test.log for details" || (echo "Tests failed! See .log/test.log for details" && exit 1)

short: ## Run short tests only
	@go test -short ./...

lint: ## Run golangci-lint
	@golangci-lint run ./...

act: ## Run GitHub Actions tests locally with act (logs to .log/act.log)
	@mkdir -p .log
	@echo "Running GitHub Actions tests locally with act..."
	@echo "Note: TestDeployment will be skipped (docker-in-docker not supported in act)"
	@echo "Output will be saved to .log/act.log"
	@act -j test --container-architecture linux/amd64 --matrix platform:ubuntu-latest > .log/act.log 2>&1 && echo "Tests passed! See .log/act.log for details" || (echo "Tests failed! See .log/act.log for details" && exit 1)

clean: ## Remove built binaries
	@rm -fv $(binaries)

latencies: ## Generate latency matrix from CSV file (use CSV=file.csv to specify)
	@echo "Generating Latency Matrix using $(CSV)"
	@go run cmd/latencygen/main.go -file "$(CSV)"

wonderproxy: ## Generate latency matrix from wonderproxy.csv
	@$(MAKE) latencies CSV=wonderproxy.csv

aws: ## Generate latency matrix from aws.csv
	@$(MAKE) latencies CSV=aws.csv

%.pb.go %_gorums.pb.go : %.proto
	protoc -I=.:$(proto_include) \
		--go_out=paths=source_relative:. \
		--gorums_out=paths=source_relative:. \
		$<
