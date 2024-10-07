#!/bin/bash
set -ex

TEMPDIR=`mktemp -d`
trap "rm -rf $TEMPDIR" EXIT
cd "$TEMPDIR"

ydls -server -listen :1234 -config "$CONFIG" &
# wait until ready
curl --retry-connrefused --retry 5 http://0:1234/

curl -sOJ "http://0:1234/mp3+1s/https://vimeo.com/454525548"
ffprobe -show_format -hide_banner -i "Sample Video - 3 minutemp4.mp4.mp3" 2>&1 | grep format_name=mp3

kill %1
