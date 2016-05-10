package ffmpeg

import (
	"strings"
	"testing"
)

const ffprobeOutput = `{
    "streams": [
        {
            "index": 0,
            "codec_name": "h264",
            "codec_long_name": "H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10",
            "profile": "High",
            "codec_type": "video",
            "codec_time_base": "1001/48000",
            "codec_tag_string": "[0][0][0][0]",
            "codec_tag": "0x0000",
            "width": 1280,
            "height": 720,
            "has_b_frames": 2,
            "sample_aspect_ratio": "1:1",
            "display_aspect_ratio": "16:9",
            "pix_fmt": "yuv420p",
            "level": 41,
            "color_range": "tv",
            "color_space": "bt709",
            "chroma_location": "left",
            "r_frame_rate": "24000/1001",
            "avg_frame_rate": "24000/1001",
            "time_base": "1/1000",
            "start_pts": 0,
            "start_time": "0.000000",
            "bits_per_raw_sample": "8",
            "disposition": {
                "default": 1,
                "dub": 0,
                "original": 0,
                "comment": 0,
                "lyrics": 0,
                "karaoke": 0,
                "forced": 0,
                "hearing_impaired": 0,
                "visual_impaired": 0,
                "clean_effects": 0,
                "attached_pic": 0
            },
            "tags": {
                "LANGUAGE": "eng"
            }
        },
        {
            "index": 1,
            "codec_name": "ac3",
            "codec_long_name": "ATSC A/52A (AC-3)",
            "codec_type": "audio",
            "codec_time_base": "1/48000",
            "codec_tag_string": "[0][0][0][0]",
            "codec_tag": "0x0000",
            "sample_fmt": "fltp",
            "sample_rate": "48000",
            "channels": 6,
            "channel_layout": "5.1(side)",
            "bits_per_sample": 0,
            "dmix_mode": "-1",
            "ltrt_cmixlev": "-1.000000",
            "ltrt_surmixlev": "-1.000000",
            "loro_cmixlev": "-1.000000",
            "loro_surmixlev": "-1.000000",
            "r_frame_rate": "0/0",
            "avg_frame_rate": "0/0",
            "time_base": "1/1000",
            "start_pts": 0,
            "start_time": "0.000000",
            "bit_rate": "384000",
            "disposition": {
                "default": 1,
                "dub": 0,
                "original": 0,
                "comment": 0,
                "lyrics": 0,
                "karaoke": 0,
                "forced": 0,
                "hearing_impaired": 0,
                "visual_impaired": 0,
                "clean_effects": 0,
                "attached_pic": 0
            }
        }
    ],
    "format": {
        "filename": "test.mkv",
        "nb_streams": 2,
        "nb_programs": 0,
        "format_name": "matroska,webm",
        "format_long_name": "Matroska / WebM",
        "start_time": "0.000000",
        "duration": "2404.277000",
        "size": "1355656107",
        "bit_rate": "4510815",
        "probe_score": 100,
        "tags": {
            "ENCODER": "mkv 2.1.1 AVS"
        }
    }
}`

func TestFFProbe(t *testing.T) {
	pi, err := probeInfoParse(strings.NewReader(ffprobeOutput))
	if err != nil {
		t.Fatal("failed to parse ffprobe output")
	}
	if pi.FormatName() != "matroska" {
		t.Fatalf("FormatName should be matroska, is %s", pi.FormatName())
	}
	if pi.ACodec() != "ac3" {
		t.Fatalf("ACodec should be ac3, is %s", pi.ACodec())
	}
	if pi.VCodec() != "h264" {
		t.Fatalf("VCodec should be h264, is %s", pi.VCodec())
	}
}
