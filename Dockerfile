FROM golang:1.10-stretch AS ydls-builder
ENV YDL_VERSION=2018.04.16
ENV CONFIG=/etc/ydls.json

RUN \
  curl -L -o /usr/local/bin/youtube-dl https://yt-dl.org/downloads/$YDL_VERSION/youtube-dl && \
  chmod a+x /usr/local/bin/youtube-dl
COPY --from=mwader/static-ffmpeg:3.4.2 /* /usr/local/bin/

COPY cmd /go/src/github.com/wader/ydls/cmd
COPY internal /go/src/github.com/wader/ydls/internal
COPY .git /go/src/github.com/wader/ydls/.git
COPY ydls.json /etc

WORKDIR /go/src/github.com/wader/ydls

RUN TEST_FFMPEG=1 TEST_YOUTUBEDL=1 TEST_NETWORK=1 go test -v -cover -race ./...
RUN go install -installsuffix netgo -tags netgo -ldflags "-X main.gitCommit=$(git describe --always)" ./cmd/ydls
RUN \
  ldd /go/bin/ydls | grep -q "not a dynamic executable" && \
  cmd/ydls/ydls_server_test.sh && \
  cmd/ydls/ydls_get_test.sh

FROM alpine:3.7
LABEL maintainer="Mattias Wadman mattias.wadman@gmail.com"
ENV PORT=8080
ENV LISTEN=:$PORT
ENV CONFIG=/etc/ydls.json

RUN apk add --no-cache \
  ca-certificates \
  tini \
  python \
  rtmpdump \
  mplayer
COPY --from=mwader/static-ffmpeg:3.4.2 /* /usr/local/bin/
COPY --from=ydls-builder \
  /go/bin/ydls \
  /usr/local/bin/youtube-dl \
  /usr/local/bin/
COPY entrypoint.sh /usr/local/bin
COPY ydls.json /etc

# make sure all binaries work
RUN \
  youtube-dl --version && \
  ydls -version

USER nobody
EXPOSE $PORT/tcp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
