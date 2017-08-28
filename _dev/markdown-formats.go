package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/wader/ydls/ydls"
)

func main() {
	ydls, _ := ydls.NewFromFile(os.Args[1])

	fmt.Print("|Format name|Container|Audio codecs|Video codecs|\n")
	fmt.Print("|-|-|-|-|\n")
	for _, f := range *ydls.Formats {
		fmt.Printf("|%s|%s|%s|%s|\n",
			f.Name,
			strings.Join(f.Formats, ", "),
			strings.Join(f.ACodecs.CodecNames(), ", "),
			strings.Join(f.VCodecs.CodecNames(), ", "),
		)
	}
}
