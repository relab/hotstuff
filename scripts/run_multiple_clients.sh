#!/usr/bin/env bash

if [ "$#" -lt 5 ]; then
	echo "Usage: $0 [num clients] [hostname] [client id] [request rate] [payload size] [maxinflight] [exit after]"
	exit 1
fi

num_clients="$1"
hostname="$2"
client_id="$3"
request_rate="$4"
payload_size="$5"
maxinflight="$6"
exit_after="$7"

for (( i = 0 ; i < $num_clients ; i++ )); do
	id=$(( client_id * num_clients + (i - num_clients) ))
	./hotstuffclient \
	--benchmark \
	--self-id "$id" \
	--max-inflight "$maxinflight" \
	--rate-limit "$request_rate" \
	--payload-size "$payload_size" \
	--exit-after "$exit_after" \
	> "/tmp/results/$hostname-c$id.out" &
done

for (( i = 0 ; i < $num_clients ; i++ )); do
	wait
done
