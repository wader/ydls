#!/bin/bash

# this scripts is used to auto update youtube-dl and ffmpeg versions
# run with GIT_SSH_COMMAND="ssh -i sshkey-ydls" ./update.sh

while true ; do
  git pull

  FFMPEG_LATEST=$(git ls-remote -t https://github.com/FFmpeg/FFmpeg | cut -f 2 | grep '^refs/tags/n' | grep -v -- -dev | grep -v {} | cut -d '/' -f 3 | sort -t. -k 1.2,1nr -k 2,2nr -k 3,3nr -k 4,4nr | head -n 1)
  YDL_LATEST=$(git ls-remote -t https://github.com/rg3/youtube-dl/ | cut -f 2 | grep -v {} | sort -nr | head -n 1 | cut -d '/' -f 3)

  echo ffmpeg $FFMPEG_LATEST
  echo youtube-dl $YDL_LATEST

  sed -i.bak "s/FFMPEG_VERSION=.*/FFMPEG_VERSION=$FFMPEG_LATEST/g" Dockerfile
  rm -f Dockerfile.bak
  if ! git diff --quiet Dockerfile ; then
    git add Dockerfile
    git commit -m "Update ffmpeg to $FFMPEG_LATEST"
  fi

  sed -i.bak "s/YDL_VERSION=.*/YDL_VERSION=$YDL_LATEST/g" Dockerfile
  rm -f Dockerfile.bak
  if ! git diff --quiet Dockerfile ; then
    git add Dockerfile
    git commit -m "Update youtube-dl to $YDL_LATEST"
  fi

  git push origin master

  sleep 86400 # sleep for a day
done
