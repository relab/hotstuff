#!/usr/bin/env bash

# default values
pacemaker="fixed"
ips=()
peer_port="30000"
client_port="40000"
keypath="."
dest="."

usage() {
	echo "Usage: $0 [options] [destination]"
	echo
	echo "Options:"
	echo "	--pacemaker <fixed|round-robin> : Specify the type of pacemaker"
	echo "	--ips <';' separated list>      : List of IP addresses to use"
	echo "	--client-port <port>            : The port that clients should use to connect to servers"
	echo "	--peer-port <port>              : The port that replicas should use to connect to each other"
	echo "	--keypath <folder>              : Path to the keys relative to the working directory"
	echo "	--keygen                        : If present, the keys will be generated in 'keypath'"
	echo
	echo "If no destination is given, the files are saved relative to the working directory."
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
		--peer-port)
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
		--*)
			echo "Unknown option '$1'."
			exit 1
			;;
		*)
			break 2
			;;
	esac
done

# check for destination
if [ -n "$1" ]; then
	dest="$1"
	mkdir -p "$dest"
fi

# generate keys
if [ -n "$keygen" ]; then
	if [ -z "$keypath" ]; then
		keypath="$dest"
	fi
	if [ ! -f cmd/hotstuffkeygen/hotstuffkeygen ]; then
		echo "hotstuffkeygen binary not built. Running make..."
		make
	fi
	mkdir -p "$keypath"
	cmd/hotstuffkeygen/hotstuffkeygen -p \*.key -n ${#ips[@]} "$keypath"
fi

# create main config file
file="$dest/hotstuff.toml"
:> "$file"

echo -e "pacemaker = \"$pacemaker\"" >> "$file"

if [ "$pacemaker" = "fixed" ]; then
	echo "leader-id = 1" >> "$file"
elif [ "$pacemaker" = "round-robin" ]; then
	echo "view-change = 100" >> "$file"
	echo -n "leader-schedule = [" >> "$file"
	for i in "${!ips[@]}"; do
		echo -n "$((i+1))," >> "$file"
	done
	echo "]" >> "$file"
fi

echo >> "$file"

for i in "${!ips[@]}"; do
	write_replica "$file" "$((i+1))" "${ips[$i]}"
	write_replica_config "$dest/hotstuff_$((i+1)).toml" "$((i+1))"
done
