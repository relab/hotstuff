#!/usr/bin/env bash

# kill all children on exit
trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

go build .

./hotstuffbench --self-id 2 --keyfile keys/r2.key --config base_perf.toml --batch-size 100 --cpuprofile cpu2.out & 
./hotstuffbench --self-id 3 --keyfile keys/r3.key --config base_perf.toml --batch-size 100 --cpuprofile cpu3.out & 
./hotstuffbench --self-id 4 --keyfile keys/r4.key --config base_perf.toml --batch-size 100 --cpuprofile cpu4.out & 

./hotstuffbench --self-id 1 --keyfile keys/r1.key --config base_perf.toml --batch-size 100 --cpuprofile cpu1.out

killall hotstuff
