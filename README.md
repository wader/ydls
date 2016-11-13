### youtube-dl HTTP service

HTTP service for [youtube-dl](https://yt-dl.org) that downloads media for
the requested URL and transmuxes and transcodes to request format if needed.

### Usage

#### Docker

Pull `mwader/ydls` or build image using Dockerfile and run a container. Depending on
your setup you should publish port 8080 somehow.

#### Installing

Run `go get github.com/wader/ydls/...` this  will install `ydls-server` and
`ydls-get`. Make sure you have ffmpeg, youtube-dl, rtmpdump and mplayer
installed and in path.

Copy and edit [formats.json](formats.json) to match your ffmpeg builds
supported formats and codecs.

Start with `ydls-server -formats /path/to/formats.json` and it default will listen
on port 8080.

### Endpoints

Download in best format:  
`GET /<URL>`  
`GET /?url=<URL>`  

Download and make sure media is in specified format:  
`GET /<format>/<URL>`  
`GET /?format=<format>&url=<URL>`

`URL` is a URL that youtube-dl can handle (if schema is missing `http://` is assumed).

`format` depends on [formats.json](formats.json) but docker image supports mp3, m4a,
ogg, mp4, webm and mkv.

Examples:

Download and make sure media is in mp3 format:  
`http://ydls-host/mp3/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Download and make sure media is in webm format:  
`http://ydls-host/webm/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Download in best format:  
`http://ydls-host/https://www.youtube.com/watch?v=cF1zJYkBW4A`

### Can't youtube-dl already do this?

Yes sort of. It can do some transmuxing and transcoding but it's done post-download
so it can take a lot of time before you get any data. That did not work that great
with the clients and usage I wanted. Also ydls have some extra logic for how to
download, e.g. if you want media in mp3 format it will look for formats in this
order:

- mp3 audio and no video in bit rate order
- mp3 audio and any video codec in bit rate order and use only mp3 audio
- any audio and no video codec in bit rate order and transcode to mp3
- any audio and video codec in bit rate order and transcode to mp3

If requested format includes video same logic is used for video. In the most
complex case that means two formats can be download concurrently and being
transmuxed and transcoded while streaming.

There is also ID3v2 support and logic to prefer formats suitable for streaming.

### Formats config

```javascript
[
  {
    "Name": "", // Format name in endpoint
    "Formats": [], // Valid container formats. First in list is used for muxing
    "FormatFlags": [], // Global format flags
    // Zero or more valid audio codecs. First in list is used if transcoding is needed
    "ACodecs": {
      "Codec": "", // Codec name
      "CodecFlags": [], // Codec flags
      "FormatFlags": [] // Format flags
    },
    // Zero or more valid video codecs. First in list is used if transcoding is needed
    "VCodecs": {
      "Codec": "", // Codec name
      "CodecFlags": [], // Codec flags
      "FormatFlags": [] // Format flags
    },
    "Prepend": "", // Can currently be "id3v2" to append ID3v2 tag
    "Ext": "", // Filename extension
    "MIMEType": "" // MIME type
  },
  // More formats...
]
```

### Tricks and known issues

Download with curl and save to filename provided by response header:

`curl -OJ http://ydls-host/mp3/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Docker image can download from command line. This will download in mp3 format
to current directory:

`docker run --rm -v "$PWD:/go" --user=root <ydls-image> https://www.youtube.com/watch?v=cF1zJYkBW4A mp3`

youtube-dl URL can point to a plain media file.

If you run the service using some cloud services you might run into geo-restriction
issues with some sites like youtube.

### TODO

- youtubedl info, just url no formats?
- X-Remote IP header?
- seccomp and chroot things
- Auto update youtube-dl somehow?

### License

ydls is licensed under the MIT license. See [LICENSE](LICENSE) for the full license text.
