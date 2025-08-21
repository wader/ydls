#!/bin/bash
set -ex

TEMPDIR=`mktemp -d`
trap "rm -rf $TEMPDIR" EXIT
cd "$TEMPDIR"

ydls -server -listen :1234 -config "$CONFIG" &
# wait until ready
curl --retry-connrefused --retry 5 http://0:1234/

curl -sOJ "http://0:1234/mp3+1s/https://media.ccc.de/v/blinkencount"
ffprobe -show_format -hide_banner -i 'Blinkencount.mp3' 2>&1 | grep format_name=mp3

kill %1
