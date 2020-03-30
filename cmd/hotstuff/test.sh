#!/usr/bin/env bash

# kill all children on exit
trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

go build .

export HOTSTUFF_LOG=1

TIMEOUT="1800"
NEWVIEW="2000"
if [[ "$1" == "debug" ]]; then
	TIMEOUT="$((1000*10))"
	NEWVIEW="$((1000*10))"
fi

COMMANDS="big.txt"

COMMON_ARGS="--leader-id 1 --connect-timeout 5000 --newview-timeout $NEWVIEW  --waitduration 1000 --timeout $TIMEOUT"

REPLICA1_ARG="--self-id 1 --keyfile keys/r1.key $COMMON_ARGS --commands 1.in"
REPLICA2_ARG="--self-id 2 --keyfile keys/r2.key $COMMON_ARGS --commands 2.in"
REPLICA3_ARG="--self-id 3 --keyfile keys/r3.key $COMMON_ARGS --commands 3.in"
REPLICA4_ARG="--self-id 4 --keyfile keys/r4.key $COMMON_ARGS --commands 4.in"

if [[ "$1" == "record" ]]; then
	if [ $(cat /proc/sys/kernel/perf_event_paranoid) -gt "1" ]; then
		sudo sysctl kernel.perf_event_paranoid=1
		if [ "$?" -ne "0" ]; then
			echo "Failed to apply kernel parameter"
			exit 1
		fi
	fi
	mkdir -p rr
	export _RR_TRACE_DIR=rr
	rr record ./hotstuff $REPLICA1_ARG >1.out &
	rr record ./hotstuff $REPLICA2_ARG >2.out &
	rr record ./hotstuff $REPLICA3_ARG >3.out &
	rr record ./hotstuff $REPLICA4_ARG >4.out &
	if [[ "$2" == "fail" ]]; then 
		sleep 2s
		echo "killing replica 4..."
		kill $!
		wait; wait; wait;
		exit
	fi
	wait; wait; wait; wait; 
	exit
fi

./hotstuff $REPLICA3_ARG >3.out &
./hotstuff $REPLICA4_ARG >4.out &

# start the leader last
if [[ "$2" == "leader" ]]; then
	./hotstuff $REPLICA2_ARG >2.out &
	dlv debug --headless --listen=:2345 --api-version=2 -- $REPLICA1_ARG
elif [[ "$2" == "replica" ]]; then
	./hotstuff $REPLICA1_ARG >1.out &
	dlv debug --headless --listen=:2345 --api-version=2 -- $REPLICA2_ARG
elif [[ "$1" == "fail" ]]; then
	./hotstuff $REPLICA2_ARG >2.out &
	./hotstuff $REPLICA1_ARG >1.out &
	PID=$!
	sleep 2s
	echo "Killing replica 1..."
	kill $PID
else
	./hotstuff $REPLICA2_ARG >2.out &
	./hotstuff $REPLICA1_ARG >1.out
fi

wait; wait; wait;
