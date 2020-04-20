#!/usr/bin/env bash

trap 'trap - SIGTERM && kill -- -$$' SIGINT SIGTERM EXIT

bin=cmd/hotstuffserver/hotstuffserver

$bin --self-id 1 --privkey keys/r1.key --cpuprofile cpuprofile.out --memprofile memprofile.out "$@" > 1.out &
$bin --self-id 2 --privkey keys/r2.key "$@" > 2.out &
$bin --self-id 3 --privkey keys/r3.key "$@" > 3.out &
$bin --self-id 4 --privkey keys/r4.key "$@" > 4.out &

wait; wait; wait; wait;
