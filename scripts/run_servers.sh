#!/usr/bin/env bash

trap 'trap - SIGTERM && kill -- -$$' SIGINT SIGTERM EXIT

bin=cmd/hotstuffserver/hotstuffserver

if [[ "$1" == "record" ]]; then
	if [ "$(cat /proc/sys/kernel/perf_event_paranoid)" -gt "1" ]; then
		sudo sysctl kernel.perf_event_paranoid=1 || (echo "Failed to apply kernel parameter"; exit 1)
	fi

	export _RR_TRACE_DIR=rr

	rr record $bin --self-id 1 --privkey keys/r1.key "$@" > 1.out &
	rr record $bin --self-id 2 --privkey keys/r2.key "$@" > 2.out &
	rr record $bin --self-id 3 --privkey keys/r3.key "$@" > 3.out &
	rr record $bin --self-id 4 --privkey keys/r4.key "$@" > 4.out &
	
	wait; wait; wait; wait;
	exit 0
fi

$bin --self-id 1 --privkey keys/r1.key --cpuprofile cpuprofile.out --memprofile memprofile.out "$@" > 1.out &
$bin --self-id 2 --privkey keys/r2.key "$@" > 2.out &
$bin --self-id 3 --privkey keys/r3.key "$@" > 3.out &
$bin --self-id 4 --privkey keys/r4.key "$@" > 4.out &

wait; wait; wait; wait;
