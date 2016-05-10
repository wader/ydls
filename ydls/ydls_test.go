package ydls

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wader/ydls/youtubedl"
)

func TestFindBestFormats(t *testing.T) {
	splitOrEmpty := func(s string) []string {
		if s == "" {
			return []string{}
		}
		return strings.Split(s, ",")
	}

	test := func(Formats []*youtubedl.Format, acodecs string, vcodecs string, aFormatID string, vFormatID string) error {
		aFormat, vFormat := findBestFormats(
			Formats,
			&Format{
				ACodecs: prioStringSet(splitOrEmpty(acodecs)),
				VCodecs: prioStringSet(splitOrEmpty(vcodecs)),
			},
		)

		if (aFormat == nil && aFormatID != "") ||
			(aFormat != nil && aFormat.FormatID != aFormatID) ||
			(vFormat == nil && vFormatID != "") ||
			(vFormat != nil && vFormat.FormatID != vFormatID) {
			gotAFormatID := ""
			if aFormat != nil {
				gotAFormatID = aFormat.FormatID
			}
			gotVFormatID := ""
			if vFormat != nil {
				gotVFormatID = vFormat.FormatID
			}
			return fmt.Errorf(
				"%v %v, expected aFormatID=%v vFormatID=%v, gotAFormatID=%v gotVFormatID=%v",
				acodecs, vcodecs,
				aFormatID, vFormatID, gotAFormatID, gotVFormatID,
			)
		}

		return nil
	}

	ydlFormats := []*youtubedl.Format{
		{FormatID: "1", Protocol: "http", NormACodec: "mp3", NormVCodec: "h264", NormBR: 1},
		{FormatID: "2", Protocol: "http", NormACodec: "", NormVCodec: "h264", NormBR: 2},
		{FormatID: "3", Protocol: "http", NormACodec: "aac", NormVCodec: "", NormBR: 3},
		{FormatID: "4", Protocol: "http", NormACodec: "vorbis", NormVCodec: "", NormBR: 4},
	}

	for _, c := range []struct {
		ydlFormats []*youtubedl.Format
		aCodecs    string
		vCodecs    string
		aFormatID  string
		vFormatID  string
	}{
		{ydlFormats, "mp3", "h264", "1", "1"},
		{ydlFormats, "mp3", "", "1", ""},
		{ydlFormats, "aac", "", "3", ""},
		{ydlFormats, "aac", "h264", "3", "2"},
		{ydlFormats, "opus", "", "4", ""},
		{ydlFormats, "opus", "v9", "4", "2"},
	} {
		if err := test(c.ydlFormats, c.aCodecs, c.vCodecs, c.aFormatID, c.vFormatID); err != nil {
			t.Error(err)
		}
	}

}
