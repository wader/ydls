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
    libfaac-dev \
    libx264-dev \
    rtmpdump \
    mplayer \
    && \
  apt-get clean

RUN \
  git clone https://github.com/FFmpeg/FFmpeg.git && \
  (cd FFmpeg && \
    git checkout release/3.1 && \
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
      --enable-libvpx \
      --enable-encoder=libvpx_vp9 \
      --enable-libopus \
      --enable-encoder=libopus \
      --enable-libfaac \
      --enable-encoder=libfaac \
      --enable-libx264 \
      --enable-encoder=libx264 \
      && \
    make && \
    make install) && \
  rm -rf FFmpeg && \
  ldconfig

# keep in sync with youtubedl/test/sync version
RUN \
  curl -L -o /usr/local/bin/youtube-dl https://yt-dl.org/downloads/2016.11.04/youtube-dl && \
  chmod a+x /usr/local/bin/youtube-dl

RUN \
  curl -L -o /usr/local/bin/tini https://github.com/krallin/tini/releases/download/v0.9.0/tini && \
  chmod a+x /usr/local/bin/tini

COPY . /go/src/github.com/wader/ydls/
COPY formats.json /etc/
COPY entrypoint.sh /usr/local/bin
RUN \
  go test github.com/wader/ydls/... && \
  go install github.com/wader/ydls/... && \
  cp /go/bin/* /usr/local/bin && \
  go clean -r github.com/wader/ydls/... && \
  rm -rf /go/*

USER nobody
EXPOSE 8080/tcp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
