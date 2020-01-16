#!/usr/bin/env bash

set -e

go build

LEADER_LISTEN="42071"
REPLICA1_LISTEN="42072"
REPLICA2_LISTEN="42073"
REPLICA3_LISTEN="42074"

LEADER_OUT="leader.out"
REPLICA1_OUT="replica1.out"
REPLICA2_OUT="replica2.out"
REPLICA3_OUT="replica3.out"

ADDR="127.0.0.1"

LEADER_ADDRESSES="$ADDR:$REPLICA1_LISTEN;$ADDR:$REPLICA2_LISTEN;$ADDR:$REPLICA3_LISTEN"

# start leader
./basichotstuff -leader=true -commands=./main.go -addresses=$LEADER_ADDRESSES -listen-port=$LEADER_LISTEN >$LEADER_OUT 2>&1 &

# start replicas
./basichotstuff -leader-address=$ADDR:$LEADER_LISTEN -listen-port=$REPLICA1_LISTEN >$REPLICA1_OUT 2>&1 &
./basichotstuff -leader-address=$ADDR:$LEADER_LISTEN -listen-port=$REPLICA2_LISTEN >$REPLICA2_OUT 2>&1 &
./basichotstuff -leader-address=$ADDR:$LEADER_LISTEN -listen-port=$REPLICA3_LISTEN >$REPLICA3_OUT 2>&1 &
