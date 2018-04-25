#!/bin/bash

# this scripts is used to auto update youtube-dl and ffmpeg versions
# run with GIT_SSH_COMMAND="ssh -i sshkey-ydls" ./update.sh

while true ; do
  git pull

  YDL_LATEST=$(git ls-remote -t https://github.com/rg3/youtube-dl/ | cut -f 2 | grep -v {} | sort -nr | head -n 1 | cut -d '/' -f 3)

  echo youtube-dl $YDL_LATEST

  if [ "$YDL_LATEST" != "" ] ; then

    sed -i.bak "s/YDL_VERSION=.*/YDL_VERSION=$YDL_LATEST/g" Dockerfile
    rm -f Dockerfile.bak
    if ! git diff --quiet Dockerfile ; then
      git add Dockerfile
      git commit -m "Update youtube-dl to $YDL_LATEST"
    fi

    git push origin master

  fi

  sleep 86400 # sleep for a day
done
