#!/usr/bin/env bash

run_benchmark() {
	mkdir -p "$1"
	ansible-playbook -i scripts/hotstuff.gcp.yml scripts/throughputvslatency.yml \
		-e "destdir='$1' rate='$2' batch_size='$3' payload='$4' time='$5'"
}

# Exit if anything fails
set -e

basedir="$1/throughputvslatency-$(date +%Y%m%d%H%M%S)"
mkdir -p "$basedir"

# how many times to repeat each benchmark
for n in {1..1}; do

	batch=100
	payload=0
	for t in {1000,2000,3000,5000,7000,10000,11000}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	done

	batch=400
	payload=0
	for t in {17000,20000,21000,23000,26000,28000}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	done

	batch=800
	payload=0
	for t in {23000,25000,27000,30000,32000,35000}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	done

done
