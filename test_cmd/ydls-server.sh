#!/bin/bash
set -ex

TEMPDIR=`mktemp -d`
trap "rm -rf $TEMPDIR" EXIT
cd "$TEMPDIR"

ydls-server -listen :1234 -formats "$FORMATS" &
sleep 1
curl -OJ "http://0:1234/mp3/https://www.youtube.com/watch?v=C0DPdy98e4c"
ffprobe -hide_banner -i "TEST VIDEO.mp3" 2>&1 | grep mp3
kill %1
