#!/bin/bash

join () {
	local IFS="$1"
	shift
	echo "$*"
}

num_hosts=4

declare -A hosts

for ((i=1; i<=num_hosts; i++)); do
	hosts[$i]="hotstuff_worker_$i"
done


if [ ! -f "./id" ]; then
	ssh-keygen -t ed25519 -C "hotstuff-test" -f "./id" -N ""
fi

compose_args="--project-name=hotstuff"

docker-compose $compose_args up -d --build --scale worker=4

docker-compose $compose_args exec -T controller /bin/sh -c "ssh-keyscan -H $(join ' ' "${hosts[@]}") >> ~/.ssh/known_hosts" &>/dev/null
docker-compose $compose_args exec -T controller /bin/sh -c "hotstuff run --hosts '$(join ',' "${hosts[@]}")' --config ./example_config.toml --log-level info"
exit_code="$?"

docker-compose $compose_args down

exit $exit_code
