#!/usr/bin/env bash

run_benchmark() {
	mkdir -p "$1"
	ansible-playbook -i scripts/hotstuff.gcp.yml scripts/throughputvslatency.yml \
		-e "destdir='$1' rate='$2' batch_size='$3' payload='$4' maxinflight='$5' time='$6' num_clients='$7'"
}

# Exit if anything fails
set -e

basedir="$1/throughputvslatency-$(date +%Y%m%d%H%M%S)"
mkdir -p "$basedir"

# how many times to repeat each benchmark
for n in {1..1}; do

	batch=100
	payload=0
	for t in {5000,7500,10000,12500,15000,17500,20000,22500,23500,25000,27500}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 325 60 1
	done

	batch=200
	payload=0
	for t in {17500,20000,22500,25000,27500,30000,33250,35000,36000,37500,40000}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 650 60 1
	done

	batch=400
	payload=0
	for t in {30000,35000,40000,42500,45000,47500,50000,50500,51000,52500,55000}; do
		run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 1300 60 1
	done

	# PAYLOAD

	# batch=400
	# payload=1024
	# for t in {500,1000,2000,3000,5000,7000,10000,12000,15000,2000}; do
	# 	run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 400 10 1
	# done

	# batch=400
	# payload=128
	# for t in {7000,10000,15000,20000,25000,27500,30000,35000,40000,45000}; do
	# 	run_benchmark "$basedir/b$batch-p$payload/$n/t$t" "$t" "$batch" "$payload" 1000 10 1
	# done

done
