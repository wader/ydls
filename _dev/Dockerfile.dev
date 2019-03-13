FROM golang:1.12-stretch
COPY --from=ydls /usr/local/bin/* /usr/local/bin/
RUN \
  apt-get update && \
  apt-get -qy install \
  less \
  jq \
  bsdmainutils \
  python \
  python-crypto \
  rtmpdump \
  mplayer

WORKDIR /src
ENTRYPOINT bash
