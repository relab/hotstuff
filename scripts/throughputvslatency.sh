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
	for t in {1,2,3,5,7,10,11}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	done

	batch=400
	payload=0
	for t in {17,20,21,23,26,28}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	done

	batch=800
	payload=0
	for t in {23,25,27,30,32,35}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	done

done
