#!/usr/bin/env bash

# set -e

go build

LEADER_LISTEN="42071"
REPLICA1_LISTEN="42072"
REPLICA2_LISTEN="42073"
REPLICA3_LISTEN="42074"

LEADER_OUT="leader.out"
REPLICA1_OUT="replica1.out"
REPLICA2_OUT="replica2.out"
REPLICA3_OUT="replica3.out"

TIMEOUT="-timeout=10"

ADDR="127.0.0.1"

LEADER_ADDRESSES="$ADDR:$REPLICA1_LISTEN;$ADDR:$REPLICA2_LISTEN;$ADDR:$REPLICA3_LISTEN"

LEADER_ARGS="-leader=true -commands=./main.go -addresses=$LEADER_ADDRESSES -listen-port=$LEADER_LISTEN $TIMEOUT"
REPLICA1_ARGS="-leader-address=$ADDR:$LEADER_LISTEN -listen-port=$REPLICA1_LISTEN $TIMEOUT"

if [[ "$1" == "debug_leader" ]]; then
	dlv debug --headless --listen=:2345 --api-version=2 -- $LEADER_ARGS &
else
	# start leader
	./basichotstuff $LEADER_ARGS  >$LEADER_OUT 2>&1 &
fi

# start replicas
./basichotstuff -leader-address=$ADDR:$LEADER_LISTEN -listen-port=$REPLICA2_LISTEN $TIMEOUT >$REPLICA2_OUT 2>&1 &
./basichotstuff -leader-address=$ADDR:$LEADER_LISTEN -listen-port=$REPLICA3_LISTEN $TIMEOUT >$REPLICA3_OUT 2>&1 &

if [[ "$1" == "debug_replica" ]]; then
	dlv debug --headless --listen=:2345 --api-version=2 -- $REPLICA1_ARGS &
else
	./basichotstuff $REPLICA1_ARGS >$REPLICA1_OUT 2>&1 &
fi

# killall basichotstuff
