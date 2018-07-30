#!/bin/bash

set -e

mkdir -p /dev/net
[ ! -e /dev/net/tun ] && mknod /dev/net/tun c 10 200

exec "$@"