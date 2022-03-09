# bump: yt-dlp /YT_DLP=([\d.-]+)/ https://github.com/yt-dlp/yt-dlp.git|/^\d/|sort
# bump: yt-dlp link "Release notes" https://github.com/yt-dlp/yt-dlp/releases/tag/$LATEST
ARG YT_DLP=2022.03.08.1
# bump: static-ffmpeg /FFMPEG_VERSION=([\d.-]+)/ docker:mwader/static-ffmpeg|/^\d/|sort
ARG FFMPEG_VERSION=5.0-1
# bump: golang /GOLANG_VERSION=([\d.]+)/ docker:golang|^1
# bump: golang link "Release notes" https://golang.org/doc/devel/release.html
ARG GOLANG_VERSION=1.17.8
# bump: alpine /ALPINE_VERSION=([\d.]+)/ docker:alpine|^3
# bump: alpine link "Release notes" https://alpinelinux.org/posts/Alpine-$LATEST-released.html
ARG ALPINE_VERSION=3.15.0

FROM mwader/static-ffmpeg:$FFMPEG_VERSION AS ffmpeg

FROM golang:$GOLANG_VERSION AS yt-dlp
ARG YT_DLP
RUN \
  curl -L https://github.com/yt-dlp/yt-dlp/releases/download/$YT_DLP/yt-dlp -o /yt-dlp && \
  chmod a+x /yt-dlp

FROM golang:$GOLANG_VERSION AS ydls-base
WORKDIR /src
RUN \
  apt-get update -q && \
  apt-get install --no-install-recommends -qy \
  python-is-python3 \
  python3-pycryptodome \
  rtmpdump \
  mplayer

COPY --from=ffmpeg /ffmpeg /ffprobe /usr/local/bin/
COPY --from=yt-dlp /yt-dlp /usr/local/bin/

FROM ydls-base AS ydls-dev
RUN \
  apt-get install --no-install-recommends -qy \
  less \
  jq \
  bsdmainutils

FROM ydls-base AS ydls-builder
COPY go.mod go.sum /src/
COPY cmd /src/cmd
COPY internal /src/internal
COPY ydls.json /src
COPY ydls.json /etc

# hack to conditionally get git commit if possible 
COPY Dockerfile .git* /src/.git/
RUN (git describe --always 2>/dev/null || echo nogit) > .GIT_COMMIT

RUN \
  CONFIG=/src/ydls.json \
  TEST_EXTERNAL=1 \
  go test -v -cover -race ./...

RUN \
  go install \
  -installsuffix netgo \
  -tags netgo \
  -ldflags "-X main.gitCommit=$(cat .GIT_COMMIT)" \
  ./cmd/ydls
RUN \
  CONFIG=/etc/ydls.json cmd/ydls/ydls_server_test.sh && \
  CONFIG=/etc/ydls.json cmd/ydls/ydls_get_test.sh

FROM alpine:$ALPINE_VERSION
LABEL maintainer="Mattias Wadman mattias.wadman@gmail.com"
ENV PORT=8080
ENV LISTEN=:$PORT
ENV CONFIG=/etc/ydls.json

RUN apk add --no-cache \
  ca-certificates \
  tini \
  python3 \
  py3-pycryptodome \
  rtmpdump \
  mplayer
# make python3 default python, symlink seems to be the way the official python alpine
# image does it https://github.com/docker-library/python/blob/master/3.8/alpine3.10/Dockerfile
RUN ln -s /usr/bin/python3 /usr/bin/python
COPY --from=ffmpeg /ffmpeg /ffprobe /usr/local/bin/
COPY --from=yt-dlp /yt-dlp /usr/local/bin/
COPY --from=ydls-builder /go/bin/ydls /usr/local/bin/
COPY entrypoint.sh /usr/local/bin
COPY ydls.json $CONFIG

# make sure all binaries work
RUN \
  ffmpeg -version && \
  ffprobe -version && \
  yt-dlp --version && \
  ydls -version

USER nobody
EXPOSE $PORT/tcp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
