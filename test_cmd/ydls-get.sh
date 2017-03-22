#!/bin/bash
set -ex

TEMPDIR=`mktemp -d`
trap "rm -rf $TEMPDIR" EXIT
cd "$TEMPDIR"

ydls-get -formats "$FORMATS" "https://www.youtube.com/watch?v=C0DPdy98e4c" mp3
ffprobe -show_format -hide_banner -i "TEST VIDEO.mp3" 2>&1 | grep format_name=mp3

ydls-get -formats "$FORMATS" "https://www.youtube.com/watch?v=C0DPdy98e4c"
ffprobe -show_format -hide_banner -i "TEST VIDEO.webm" 2>&1 | grep format_name=matroska,webm
