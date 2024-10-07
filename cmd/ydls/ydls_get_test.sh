#!/bin/bash
set -ex

TEMPDIR=`mktemp -d`
trap "rm -rf $TEMPDIR" EXIT
cd "$TEMPDIR"

ydls -noprogress -config "$CONFIG" "https://vimeo.com/454525548" mp3
ffprobe -show_format -hide_banner -i "Sample Video - 3 minutemp4.mp4.mp3" 2>&1 | grep format_name=mp3
