#!/usr/bin/env bash

mkdir -p ./keys

pushd ../hotstuffkeygen >/dev/null
go build .
popd >/dev/null

../hotstuffkeygen/hotstuffkeygen keys/r1.key
../hotstuffkeygen/hotstuffkeygen keys/r2.key
../hotstuffkeygen/hotstuffkeygen keys/r3.key
../hotstuffkeygen/hotstuffkeygen keys/r4.key
