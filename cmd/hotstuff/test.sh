#!/usr/bin/env bash

go build .

export HOTSTUFF_LOG=1

TIMEOUT="2000"
if [[ "$1" == "debug" ]]; then
	TIMEOUT="$((1000*10))"
fi

COMMANDS="big.txt"

LEADER_ARG="--self-id 1 --keyfile keys/r1.key --leader-id 1 --commands $COMMANDS --timeout $TIMEOUT --cpuprofile cpuprofile.out"

./hotstuff --self-id 2 --keyfile keys/r2.key --leader-id 1 --commands $COMMANDS --timeout $TIMEOUT >2.out &
./hotstuff --self-id 3 --keyfile keys/r3.key --leader-id 1 --commands $COMMANDS --timeout $TIMEOUT >3.out &
./hotstuff --self-id 4 --keyfile keys/r4.key --leader-id 1 --commands $COMMANDS --timeout $TIMEOUT >4.out &

# start the leader last
if [[ "$2" == "leader" ]]; then
	dlv debug --headless --listen=:2345 --api-version=2 -- $LEADER_ARG
else
	./hotstuff $LEADER_ARG >1.out
fi

killall hotstuff
