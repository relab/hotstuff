#!/usr/bin/env bash

keygen_path="./cmd/hotstuffkeygen"
keygen_bin="$keygen_path/hotstuffkeygen"

exit_error() {
	echo "$1" >&2
	exit 1
}

usage() {
	exit_error "Usage: $0 [path to keys] [filename containing a '*'] [how many to generate]"
}

[ "$1" = "--help" ] && usage

[ "$#" -ne "3" ] && usage

echo "$2" | grep '*' > /dev/null || \
	exit_error "The filename should contain a '*' that will be replaced with a number"

[ ! -d "$keygen_path" ] && \
	exit_error "hotstuffkeygen not found. Make sure to run this script from the project root folder."

if [ ! -f "$keygen_bin" ]; then
	echo "building hotstuffkeygen..."
	go build -o "$keygen_bin" "$keygen_path" || exit_error "Failed to build hotstuffkeygen."
fi

mkdir -p "$1" || exit_error "Failed to create target directory."

i=1
while [ "$i" -le "$3" ]; do
	"$keygen_bin" "$1/${2/'*'/$i}" || exit_error "Failed to create all keys." 
	i=$(($i + 1))
done
