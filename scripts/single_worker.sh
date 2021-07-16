#!/bin/bash

image="hotstuff_worker"

if [ ! -f "./id" ]; then
	ssh-keygen -t ed25519 -C "hotstuff-test" -f "./id" -N ""
fi

# ensure that the image is built
docker images | grep "$image" &>/dev/null \
	|| docker build -t "$image" -f "./Dockerfile.worker" ".."

container="$(docker run --rm -d -p 2020:22 -p 4000:4000 "$image")"

sleep 1s

ssh-keyscan -p 2020 127.0.0.1 localhost > known_hosts

docker logs --follow "$container"
docker rm -f "$container" &>/dev/null
