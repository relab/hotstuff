#!/bin/bash

if [ ! -f "./id" ]; then
	ssh-keygen -t ed25519 -C "hotstuff-test" -f "./id" -N ""
fi

compose_args="--project-name=hotstuff"

docker-compose $compose_args up -d --build

docker-compose $compose_args exec controller /bin/sh -c "ssh-keyscan -H hotstuff_worker_1 >> ~/.ssh/known_hosts"
docker-compose $compose_args exec controller /bin/sh -c "hotstuff run --hosts 'hotstuff_worker_1'" -e "HOTSTUFF_LOG=info"

docker-compose $compose_args down
