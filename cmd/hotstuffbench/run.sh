#!/usr/bin/env bash

# kill all children on exit
trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

go build .

ARGS="--config base_perf.toml --batch-size 100 --delay 1000"

./hotstuffbench --self-id 2 --keyfile keys/r2.key $ARGS & 
./hotstuffbench --self-id 3 --keyfile keys/r3.key $ARGS & 
./hotstuffbench --self-id 4 --keyfile keys/r4.key $ARGS & 

./hotstuffbench --self-id 1 --keyfile keys/r1.key $ARGS

killall hotstuff
