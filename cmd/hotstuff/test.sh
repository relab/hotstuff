#!/usr/bin/env bash

go build .

export HOTSTUFF_LOG=1

TIMEOUT="2000"
if [[ "$1" == "debug" ]]; then
	TIMEOUT="$((1000*10))"
fi

LEADER_ARG="--self-id 1 --keyfile keys/r1.key --leader-id 1 --commands main.go --timeout $TIMEOUT"

./hotstuff --self-id 2 --keyfile keys/r2.key --leader-id 1 --timeout $TIMEOUT &
./hotstuff --self-id 3 --keyfile keys/r3.key --leader-id 1 --timeout $TIMEOUT &
./hotstuff --self-id 4 --keyfile keys/r4.key --leader-id 1 --timeout $TIMEOUT &

# start the leader last
if [[ "$2" == "leader" ]]; then
	dlv debug --headless --listen=:2345 --api-version=2 -- $LEADER_ARG
else
	./hotstuff $LEADER_ARG
fi

killall hotstuff
