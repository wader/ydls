FFMPEG_LATEST=$(git ls-remote -t https://github.com/FFmpeg/FFmpeg | cut -f 2 | grep '^refs/tags/n' | grep -v -- -dev | grep -v {} | sort -nr | head -n 1 | cut -d '/' -f 3)
YDL_LATEST=$(git ls-remote -t https://github.com/rg3/youtube-dl/ | cut -f 2 | grep -v {} | sort -nr | head -n 1 | cut -d '/' -f 3)

echo ffmpeg $FFMPEG_LATEST
echo youtube-dl $YDL_LATEST

sed -i '' "s/FFMPEG_VERSION=.*/FFMPEG_VERSION=$FFMPEG_LATEST/g" Dockerfile
if ! git diff --quiet Dockerfile ; then
  git add Dockerfile
  git commit -m "Update ffmpeg to $FFMPEG_LATEST"
fi

sed -i '' "s/YDL_VERSION=.*/YDL_VERSION=$YDL_LATEST/g" Dockerfile
if ! git diff --quiet Dockerfile ; then
  git add Dockerfile
  git commit -m "Update youtube-dl to $YDL_LATEST"
fi
