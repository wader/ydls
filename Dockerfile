FROM debian:stretch as ffmpeg-builder
ENV FFMPEG_VERSION=n3.3.4
ENV X265_VERSION=upstream/2.5

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
    libx264-dev \
    libnuma-dev \
    cmake \
    && \
  apt-get clean

# libx265 in debian seems to not work with new versions of ffmpeg
RUN \
  git clone --branch $X265_VERSION https://anonscm.debian.org/git/pkg-multimedia/x265.git && \
  (cd x265 && \
    cd source && \
    cmake . && \
    make -j4 && \
    make install \
  )

RUN \
  git clone --branch $FFMPEG_VERSION --depth 1 https://github.com/FFmpeg/FFmpeg.git && \
  (cd FFmpeg && \
    ./configure \
      --disable-shared \
      --enable-static \
      --pkg-config-flags=--static \
      --extra-ldflags=-static \
      --extra-cflags=-static \
      --enable-gpl \
      --enable-nonfree \
      --enable-openssl \
      --disable-ffserver \
      --disable-doc \
      --disable-ffplay \
      --disable-encoders \
      --enable-encoder=aac \
      --enable-encoder=flac \
      --enable-encoder=pcm_s16le \
      --enable-libmp3lame \
      --enable-encoder=libmp3lame \
      --enable-libvorbis \
      --enable-encoder=libvorbis \
      --enable-libopus \
      --enable-encoder=libopus \
      --enable-libvpx \
      --enable-encoder=libvpx_vp8 \
      --enable-encoder=libvpx_vp9 \
      --enable-libx264 \
      --enable-encoder=libx264 \
      --enable-libx265 \
      --enable-encoder=libx265 \
      && \
    make && \
    make install) && \
  rm -rf FFmpeg && \
  ldd /usr/local/bin/ffmpeg | grep -q "not a dynamic executable" && \
  ldd /usr/local/bin/ffprobe | grep -q "not a dynamic executable" && \
  ldconfig

FROM golang:1.9-stretch as ydls-builder
ENV YDL_VERSION=2017.09.15
ENV FORMATS=/etc/formats.json

RUN \
  curl -L -o /usr/local/bin/youtube-dl https://yt-dl.org/downloads/$YDL_VERSION/youtube-dl && \
  chmod a+x /usr/local/bin/youtube-dl
COPY --from=ffmpeg-builder \
  /usr/local/bin/ffmpeg \
  /usr/local/bin/ffprobe \
  /usr/local/bin/

COPY . /go/src/github.com/wader/ydls/
COPY formats.json /etc

WORKDIR /go/src/github.com/wader/ydls

RUN TEST_FFMPEG=1 TEST_YOUTUBEDL=1 TEST_NETWORK=1 go test -v -cover -race ./...
RUN go install -installsuffix netgo -tags netgo -ldflags "-X main.gitCommit=$(git describe --always)" ./cmd/...
RUN \
  ldd /go/bin/ydls-get | grep -q "not a dynamic executable" && \
  ldd /go/bin/ydls-server | grep -q "not a dynamic executable" && \
  test_cmd/ydls-get.sh && \
  test_cmd/ydls-server.sh

FROM alpine:3.6
LABEL maintainer="Mattias Wadman mattias.wadman@gmail.com"
ENV LISTEN=:8080
ENV FORMATS=/etc/formats.json

RUN apk add --no-cache \
  ca-certificates \
  tini \
  python \
  rtmpdump \
  mplayer
COPY --from=ffmpeg-builder \
  /usr/local/bin/ffmpeg \
  /usr/local/bin/ffprobe \
  /usr/local/bin/
COPY --from=ydls-builder \
  /go/bin/ydls-server \
  /go/bin/ydls-get \
  /usr/local/bin/youtube-dl \
  /usr/local/bin/
COPY entrypoint.sh /usr/local/bin
COPY formats.json /etc

# make sure all binaries work
RUN \
  youtube-dl --version && \
  ffmpeg -version && \
  ffprobe -version && \
  ydls-get -version && \
  ydls-server -version

USER nobody
EXPOSE 8080/tcp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
