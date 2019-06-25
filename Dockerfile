ARG YDL_VERSION=2019.06.21
ARG FFMPEG_VERSION=4.1.3-2

FROM mwader/static-ffmpeg:$FFMPEG_VERSION AS ffmpeg

FROM golang:1.12-stretch AS youtube-dl
ARG YDL_VERSION
RUN \
  curl -L -o /youtube-dl https://yt-dl.org/downloads/$YDL_VERSION/youtube-dl && \
  chmod a+x /youtube-dl

FROM golang:1.12-stretch AS ydls-builder
RUN \
  apt-get update -q && \
  apt-get install -qy \
  python \
  python-crypto \
  rtmpdump \
  mplayer

COPY --from=ffmpeg /ffmpeg /ffprobe /usr/local/bin/
COPY --from=youtube-dl /youtube-dl /usr/local/bin/

WORKDIR /src
COPY go.mod /src
COPY cmd /src/cmd
COPY internal /src/internal
COPY ydls.json /src
COPY ydls.json /etc

# hack to conditionally get git commit if possible 
COPY Dockerfile .git* /src/.git/
RUN echo $(git describe --always 2>/dev/null || echo nogit) > .GIT_COMMIT

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

FROM alpine:3.10
LABEL maintainer="Mattias Wadman mattias.wadman@gmail.com"
ENV PORT=8080
ENV LISTEN=:$PORT
ENV CONFIG=/etc/ydls.json

RUN apk add --no-cache \
  ca-certificates \
  tini \
  python \
  py2-crypto \
  rtmpdump \
  mplayer
COPY --from=ffmpeg /ffmpeg /ffprobe /usr/local/bin/
COPY --from=youtube-dl /youtube-dl /usr/local/bin/
COPY --from=ydls-builder /go/bin/ydls /usr/local/bin/
COPY entrypoint.sh /usr/local/bin
COPY ydls.json $CONFIG

# make sure all binaries work
RUN \
  ffmpeg -version && \
  ffprobe -version && \
  youtube-dl --version && \
  ydls -version

USER nobody
EXPOSE $PORT/tcp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
