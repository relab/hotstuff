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
	for t in {3000,5000,7000,10000,12000,14000,17000,20000,21000,23000,24000,25000}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	done

	batch=400
	payload=0
	for t in {15000,25000,28000,30000,36000,50000,55000,60000,65000,70000,75000,80000}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	done

	batch=800
	payload=0
	for t in {36000,40000,50000,60000,70000,80000,90000,950000,100000,110000}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	done

	# batch=1200
	# payload=0
	# for t in {28000,30000,32000,34000,36000,40000,42000,44000,46000}; do
	# 	run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 10
	# done

done
