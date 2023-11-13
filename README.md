## youtube-dl HTTP service

HTTP service for [yt-dlp](https://github.com/yt-dlp/yt-dlp) that downloads media from
requested URL and streams, transmux and transcode to requested format if needed.

It can be used to convert media, create podcasts and audio only versions of media
from various site like youtube, vimeo etc.

Docker image uses [ffmpeg from wader/static-ffmpeg](https://github.com/wader/static-ffmpeg) which
can handle most format and codecs. Default configuration can transcode to these format, container and codecs:

|Format|Container|Audio|Video|Subtitle|
|-|-|-|-|-|
|alac|mp4|alac|||
|flac|flac|flac|||
|gif|gif||gif||
|m4a|mp4|aac|||
|mp3|mp3|mp3|||
|ogg|ogg|vorbis, opus|||
|wav|wav|pcm_s16le|||
|mkv|matroska|aac, flac, alac, ac3, mp3, vorbis, opus|h264, hevc, vp8, vp9, theora, av1|subrip, ass|
|mp4|mp4|aac, mp3, vorbis, flac, alac|h264, hevc, av1|mov_text|
|mxf|mxf|pcm_s16le|mpeg2video||
|ts|mpegts|aac, mp3, ac3|h264, hevc||
|webm|webm|vorbis, opus|vp8, vp9, av1|webvtt|
|rss|mp3|mp3|||

The `rss` format transforms a playlist into a RSS audio podcast.

See [ydls.json](ydls.json) for more details.

## Usage

### Run with docker

Pull `mwader/ydls` or build image using the Dockerfile. Run a container and publish
TCP port 8080 somehow.

`docker run -p 8080:8080 mwader/ydls `

### Build and install yourself

Run `go get github.com/wader/ydls/cmd/ydls` to install `ydls`.
Make sure you have ffmpeg, yt-dlp, rtmpdump and mplayer
installed and in `PATH`.

Copy and edit [ydls.json](ydls.json) to match your ffmpeg builds
supported formats and codecs.

Start with `ydls -server -config /path/to/ydls.json` and it default will listen
on port 8080.

## Endpoints

Download and make sure media is in specified format:  
`GET /<format>[+option+option...]/<URL-not-encoded>`  
`GET /?format=<format>&url=<URL>[&codec=...&codec=...&retranscode=...]`

Download in best format:  
`GET /<URL-not-encoded>`  
`GET /?url=<URL-encoded>`  

### Parameters

`format` - Format name. See table above and [ydls.json](ydls.json)  
`URL` - Any URL that [yt-dlp](https://github.com/blackjack4494/yt-dlc) can handle  
`URL-not-encoded` - Non-URL-encoded URL. The idea is to be able to simply
prepend the download URL with the ydls URL by hand without doing any encoding
(for example in the browser location bar)  
`codec` - Codec to use instead of default for format (can be specified one or two times for
audio and video codec)  
`retranscode` - Retranscode even if input codec is same as output  
`time` - Only download specificed time range. Ex: `30s`, `20m30s`, `1h20s30s` will limit
duration. `10s-30s` will seek 10 seconds and stop at 30 seconds (20 second output duration)  
`items` - If playlist only include this many items

`option` - Codec name, time range, `retranscode` or `<N>items`

### Examples

Download and make sure media is in mp3 format:  
`http://ydls/mp3/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Download using query parameters and make sure media is in mp3 format:  
`http://ydls/?format=mp3&url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DcF1zJYkBW4A`

Download and make sure media is in webm format:  
`http://ydls/webm/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Download and make sure media is in mkv format using mp3 and h264 codecs:  
`http://ydls/mkv+mp3+h264/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Download and retranscode to mp3 even if input is already mp3:  
`http://ydls/mp3+retranscode/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Download specified time range in mp3:  
`http://ydls/mp3+10s-30s/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Download in best format:  
`http://ydls/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Playlist as audio podcast with 3 latest items:  
`http://ydls/rss+3items/https://www.youtube.com/watch?list=PLtLJO5JKE5YCYgIdpJPxNzWxpMuUWgbVi`

## Tricks and known issues

For some formats the transcoded file might have zero length or duration as transcoding is done
while streaming. This is usually not a problem for most players.

Download with curl and save to filename provided by response header:

`curl -OJ http://ydls-host/mp3/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Docker image can download from command line. This will download in mp3 format
to current directory:

`docker run --rm -v "$PWD:$PWD" -w "$PWD" mwader/ydls https://www.youtube.com/watch?v=cF1zJYkBW4A mp3`

yt-dlp URL can point to a plain media file.

If you run the service using some cloud services you might run into geo-restriction
issues with some sites like youtube.

## Development

When fiddling with ffmpeg and yt-dlp related code I usually do this:

```sh
docker build --target ydls-dev -t ydls-dev . && docker run --rm -ti -v "$PWD:/$PWD" -w "$PWD" ydls-dev
```

Then inside dev container:

```sh
# run cli version
go run cmd/ydls/main.go -config ./ydls.json -debug
# run all tests
CONFIG="$PWD/ydls.json" TEST_EXTERNAL=1 go test -v -cover -race ./...
```

## TODO

- Optional stream for format? example mp4 at least video or audio?
- Download playlist as zip archive.
- JSON output?
- Refactor messy downloadFormat function. Maybe a media interface with audio, video and subtitle
implementations that select best format and return reader and mappings? should share a common
format picker so formats can be shared and not re-downloaded.
- Make download part into an interface to support more than yt-dlp.
- Bitrate factor per codec when sorting formats (prefer aac over mp3 at same bitrate etc)
- X-Remote IP header?

## License

ydls is licensed under the MIT license. See [LICENSE](LICENSE) for the full license text.
