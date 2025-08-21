#!/bin/bash
set -ex

TEMPDIR=`mktemp -d`
trap "rm -rf $TEMPDIR" EXIT
cd "$TEMPDIR"

ydls -noprogress -config "$CONFIG" "https://media.ccc.de/v/blinkencount" mp3
ffprobe -show_format -hide_banner -i 'Blinkencount.mp3' 2>&1 | grep format_name=mp3
