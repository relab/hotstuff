#!/usr/bin/env bash

# default values
prefix="hotstuff"
pacemaker="fixed"
ips=()
peer_port="30000"
client_port="40000"
keypath="keys"

usage() {
	echo "Usage: $0 [options]"
	echo
	echo "Options:"
	echo "	--prefix <prefix>               : Specify a prefix for the generated config files"
	echo "	--pacemaker <fixed|round-robin> : Specify the type of pacemaker"
	echo "	--ips <';' separated list>      : List of IP addresses to use"
	echo "	--client-port <port>            : The port that clients should use to connect to servers"
	echo "	--peer-port <port>              : The port that replicas should use to connect to each other"
	echo "	--keypath <folder>              : Path to the keys relative to the working directory"
	echo "	--keygen                        : If present, the keys will be generated in 'keypath'"
}

write_replica() {
	cat <<EOF >> "$1"
[[replicas]]
id = $2
peer-address = "$3:$peer_port"
client-address = "$3:$client_port"
pubkey = "$keypath/$2.key.pub"

EOF
}

write_replica_config() {
	cat <<EOF > "$1"
self-id = $2
privkey = "$keypath/$2.key"

EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
		--help)
			usage
			exit
			;;
		--prefix)
			prefix="$2"
			shift 2
			;;
		--pacemaker)
			if [ "$2" = "fixed" ] || [ "$2" = "round-robin" ]; then
				pacemaker="$2"
			else
				echo "Unknown pacemaker type '$2'. Use either 'fixed' or 'round-robin'." 1>&2
				exit 1
			fi
			shift 2
			;;
		--ips)
			IFS=";" read -ra ips <<< "$2"
			shift 2
			;;
		--peer_port)
			peer_port="$2"
			shift 2
			;;
		--client-port)
			client_port="$2"
			shift 2
			;;
		--keypath)
			keypath="$2"
			shift 2
			;;
		--keygen)
			keygen="1"
			shift
			;;
		*)
			"Unknown option '$1'."
			exit 1
			;;
	esac
done

# generate keys
if [ -n "$keygen" ]; then
	scripts/generate_keys.sh "$keypath" \*.key ${#ips[@]}
fi

# create main config file
file="$prefix.toml"
:> "$file"

echo -e -n "pacemaker = \"$pacemaker\"\n" > "$file"

for i in "${!ips[@]}"; do
	write_replica "$file" "$i" "${ips[$i]}"
	write_replica_config "${prefix}_${i}.toml" "$i"
done
