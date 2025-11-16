#!/bin/bash

LOG_DIR=".log/deploy-test-gorums-v0.10.0"
BASE_FILE="gorums-v0.10.0-test"
mkdir -p "$LOG_DIR"
for i in {1..50}; do
  echo "Run #$i"
  go test -v -run TestDeployment ./internal/orchestration -timeout=4m > "$LOG_DIR/${BASE_FILE}-deployment-${i}.log" 2>&1
  go test -v -run TestOrchestration ./internal/orchestration -timeout=4m > "$LOG_DIR/${BASE_FILE}-orchestration-${i}.log" 2>&1
done
