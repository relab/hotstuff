#!/usr/bin/env bash

trap 'trap - SIGTERM && kill -- -$$' SIGINT SIGTERM EXIT

bin=cmd/hotstuffserver/hotstuffserver

run_with_rr() {
	export _RR_TRACE_DIR="rr/r$1"
	mkdir -p "$_RR_TRACE_DIR"
	rr record "$bin" --self-id "$1" --privkey "keys/r$1.key" "${@:2}" > "$1".out &
}

run_with_dlv() {
	dlv debug --headless --listen=:2345 --api-version=2 ./cmd/hotstuffserver/ $@
}

if [[ "$1" == "record" ]]; then
	if [ "$(cat /proc/sys/kernel/perf_event_paranoid)" -gt "1" ]; then
		sudo sysctl kernel.perf_event_paranoid=1 || (echo "Failed to apply kernel parameter"; exit 1)
	fi

	run_with_rr 1 "${@:2}"
	run_with_rr 2 "${@:2}"
	run_with_rr 3 "${@:2}"
	run_with_rr 4 "${@:2}"

	if [ "$2" = "kill" ]; then
		sleep 2s
		kill $!
	fi
	
	wait; wait; wait; wait;
	exit 0
fi

if [[ "$1" == "debug" ]]; then
	shift 1
	run_with_dlv -- --self-id 1 --privkey keys/r1.key --cpuprofile cpuprofile.out --memprofile memprofile.out "$@" > 1.out &
else
	$bin --self-id 1 --privkey keys/r1.key --cpuprofile cpuprofile.out --memprofile memprofile.out "$@" > 1.out &
fi

$bin --self-id 2 --privkey keys/r2.key "$@" > 2.out &
$bin --self-id 3 --privkey keys/r3.key "$@" > 3.out &
$bin --self-id 4 --privkey keys/r4.key "$@" > 4.out &

if [ "$1" = "kill" ]; then
	sleep 5s
	kill $!
fi

wait; wait; wait; wait;
