package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/wader/ydls/internal/ydls"
)

func main() {
	y, _ := ydls.NewFromFile(os.Args[1])

	type formatSort struct {
		name   string
		format ydls.Format
	}
	var formats []formatSort

	for formatName, format := range y.Config.Formats {
		formats = append(formats, formatSort{formatName, format})
	}
	sort.Slice(formats, func(i int, j int) bool {
		switch a, b := len(formats[i].format.Streams), len(formats[j].format.Streams); {
		case a < b:
			return true
		case a > b:
			return false
		}
		if strings.Compare(formats[i].name, formats[j].name) < 0 {
			return true
		}
		return false
	})

	fmt.Print("|Format name|Container|Audio codecs|Video codecs|\n")
	fmt.Print("|-|-|-|-|\n")
	for _, f := range formats {
		var aCodecs []string
		var vCodecs []string
		for _, s := range f.format.Streams {
			if s.Media == ydls.MediaAudio {
				aCodecs = s.CodecNames.Strings()
			} else if s.Media == ydls.MediaVideo {
				vCodecs = s.CodecNames.Strings()
			}
		}

		firstFormat, _ := f.format.Formats.First()
		fmt.Printf("|%s|%s|%s|%s|\n",
			f.name,
			firstFormat,
			strings.Join(aCodecs, ", "),
			strings.Join(vCodecs, ", "),
		)
	}
}
