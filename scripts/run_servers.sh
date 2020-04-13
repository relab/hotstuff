#!/usr/bin/env bash

trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

BIN=cmd/hotstuffserver/hotstuffserver

$BIN --self-id 1 --privkey keys/r1.key --print-commands > 1.out &
$BIN --self-id 2 --privkey keys/r2.key --print-commands > 2.out &
$BIN --self-id 3 --privkey keys/r3.key --print-commands > 3.out &
$BIN --self-id 4 --privkey keys/r4.key --print-commands > 4.out &

wait; wait; wait; wait;
