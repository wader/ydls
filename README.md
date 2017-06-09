### youtube-dl HTTP service

HTTP service for [youtube-dl](https://yt-dl.org) that downloads media for
requested URL and transmuxes and transcodes to requested format if needed.

I personally use this mostly to create audio only versions of videos from
various site like youtube, vimeo etc.

### Usage

#### Run with docker

Pull `mwader/ydls` or build image using Dockerfile. Run a container that publishes container TCP port 8080 somehow.

`docker run --rm -p 8080:8080 mwader/ydls `

Docker image supports mp3, m4a, ogg, mp4, webm and mkv. See
[formats.json](formats.json) for details.

#### Build and install yourself

Run `go get github.com/wader/ydls/cmd/...` This  will install `ydls-server` and
`ydls-get`. Make sure you have ffmpeg, youtube-dl, rtmpdump and mplayer
installed and in path.

Copy and edit [formats.json](formats.json) to match your ffmpeg builds
supported formats and codecs.

Start with `ydls-server -formats /path/to/formats.json` and it default will listen
on port 8080.

### Endpoints

Download and make sure media is in specified format:  
`GET /<format>/<URL-not-encoded>`  
`GET /?format=<format>&url=<URL-encoded>`

Download in best format:  
`GET /<URL-not-encoded>`  
`GET /?url=<URL-encoded>`  

`format` depends on [formats.json](formats.json).

`URL` is any URL that [youtube-dl](https://yt-dl.org) can handle.
If schema is missing `http://` is assumed.

The idea with endpoints supporting `URL-not-encoded` is to be able to simply
prepend the URL with the ydls URL without doing any encoding (for example in
 the browser location bar).

Examples:

Download and make sure media is in mp3 format:  
`http://ydls-host/mp3/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Download using query parameters and make sure media is in mp3 format:  
`http://ydls-host/?format=mp3&url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DcF1zJYkBW4A`

Download and make sure media is in webm format:  
`http://ydls-host/webm/https://www.youtube.com/watch?v=cF1zJYkBW4A`

Download in best format:  
`http://ydls-host/https://www.youtube.com/watch?v=cF1zJYkBW4A`

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

### Development

When fiddling with ffmpeg and youtube-dl related code i usually build the image:  
`docker build -t ydls .`
and then test stuff from a docker instance:  
`docker run --rm -ti --entrypoint bash -v $PWD:/go/src/github.com/wader/ydls -w /go/src/github.com/wader/ydls ydls`.

### TODO

- youtubedl info, just url no formats?
- X-Remote IP header?
- seccomp and chroot things
- Auto update youtube-dl somehow?

### License

ydls is licensed under the MIT license. See [LICENSE](LICENSE) for the full license text.
