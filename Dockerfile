ARG YDL_VERSION=2019.01.02
ARG FFMPEG_VERSION=4.1

FROM mwader/static-ffmpeg:$FFMPEG_VERSION AS ffmpeg

FROM golang:1.11-stretch AS youtube-dl
ARG YDL_VERSION
RUN \
  curl -L -o /youtube-dl https://yt-dl.org/downloads/$YDL_VERSION/youtube-dl && \
  chmod a+x /youtube-dl

FROM golang:1.11-stretch AS ydls-builder
ENV CONFIG=/etc/ydls.json
COPY --from=ffmpeg /ffmpeg /ffprobe /usr/local/bin/
COPY --from=youtube-dl /youtube-dl /usr/local/bin/

WORKDIR /src
COPY go.mod /src
COPY cmd /src/cmd
COPY internal /src/internal
COPY .git /src/.git
COPY ydls.json /etc

RUN TEST_FFMPEG=1 TEST_YOUTUBEDL=1 TEST_NETWORK=1 go test -v -cover -race ./...
RUN go install -installsuffix netgo -tags netgo -ldflags "-X main.gitCommit=$(git describe --always)" ./cmd/ydls
RUN \
  ldd /go/bin/ydls | grep -q "not a dynamic executable" && \
  cmd/ydls/ydls_server_test.sh && \
  cmd/ydls/ydls_get_test.sh

FROM alpine:3.8
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
COPY ydls.json /etc

# make sure all binaries work
RUN \
  ffmpeg -version && \
  ffprobe -version && \
  youtube-dl --version && \
  ydls -version

USER nobody
EXPOSE $PORT/tcp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
