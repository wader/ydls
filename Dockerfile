FROM golang:1.7
MAINTAINER Mattias Wadman mattias.wadman@gmail.com

RUN \
  sed -i 's/main/main contrib non-free/g' /etc/apt/sources.list && \
  apt-get update && \
  apt-get -y install \
    build-essential \
    git-core \
    yasm \
    pkg-config \
    libssl-dev \
    libmp3lame-dev \
    libvorbis-dev \
    libvpx-dev \
    libopus-dev \
    libfdk-aac-dev \
    libx264-dev \
    rtmpdump \
    mplayer \
    && \
  apt-get clean

RUN \
  git clone https://github.com/FFmpeg/FFmpeg.git && \
  (cd FFmpeg && \
    git checkout release/3.2 && \
    ./configure \
      --toolchain=hardened \
      --enable-gpl \
      --enable-nonfree \
      --enable-openssl \
      --disable-ffserver \
      --disable-doc \
      --disable-ffplay \
      --disable-encoders \
      --enable-libmp3lame \
      --enable-encoder=libmp3lame \
      --enable-libvorbis \
      --enable-encoder=libvorbis \
      --enable-libopus \
      --enable-encoder=libopus \
      --enable-libvpx \
      --enable-encoder=libvpx_vp8 \
      --enable-encoder=libvpx_vp9 \
      --enable-libfdk-aac \
      --enable-encoder=libfdk_aac \
      --enable-libx264 \
      --enable-encoder=libx264 \
      && \
    make && \
    make install) && \
  rm -rf FFmpeg && \
  ldconfig

RUN \
  curl -L -o /usr/local/bin/youtube-dl https://yt-dl.org/downloads/2017.02.16/youtube-dl && \
  chmod a+x /usr/local/bin/youtube-dl

RUN \
  curl -L -o /usr/local/bin/tini https://github.com/krallin/tini/releases/download/v0.13.2/tini && \
  chmod a+x /usr/local/bin/tini

COPY . /go/src/github.com/wader/ydls/
COPY formats.json /etc/
COPY entrypoint.sh /usr/local/bin

RUN \
  TEST_FFMPEG=1 TEST_YOUTUBEDL=1 TEST_NETWORK=1 FORMATS=/etc/formats.json \
    go test -v -cover -race github.com/wader/ydls/... && \
  go install github.com/wader/ydls/cmd/... && \
  cp /go/bin/* /usr/local/bin && \
  go clean -r github.com/wader/ydls/cmd/... && \
  rm -rf /go/*

USER nobody
EXPOSE 8080/tcp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
