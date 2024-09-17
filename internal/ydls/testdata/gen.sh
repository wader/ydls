#!/bin/env bash
ffmpeg \
    -y \
    -f lavfi -i testsrc \
    -f lavfi -i sine \
    -i <(echo -e "1\n00:00:01,000 --> 00:00:05,000\nhello\n") \
    -b:v:0 256k -c:v h264 \
    -b:a:0 256k -c:a aac -ac 2\
    -c:s:0 webvtt \
    -map 0:v:0 \
    -map 1:a:0 \
    -map 2:s:0 \
    -t 10s \
    -f hls \
    -var_stream_map "v:0,a:0,s:0,language:en,sgroup:subtitle" \
    -master_pl_name master.m3u8 \
    -hls_flags discont_start+split_by_time \
    hls/stream.m3u8
