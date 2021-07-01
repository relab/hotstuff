#!/bin/bash

mkdir "$HOME/.ssh"
echo "$AUTHORIZED_KEYS" > "$HOME/.ssh/authorized_keys"
service ssh start
exec "$@"
