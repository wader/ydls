package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/wader/ydls/ydls"
)

var commit string = "dev"

var versionFlag = flag.Bool("version", false, "version")
var infoFlag = flag.Bool("info", false, "info output")
var debugFlag = flag.Bool("debug", false, "debug output")
var formatsFlag = flag.String("formats", "formats.json", "formats config file")
var indexFlag = flag.String("index", "", "index html template")
var listenFlag = flag.String("listen", ":8080", "listen address")

func init() {
	flag.Parse()

	if *versionFlag {
		fmt.Println(commit)
		os.Exit(0)
	}
	if os.Getenv("DEBUG") != "" {
		*debugFlag = true
	}
}

func main() {
	yh := &ydls.Handler{}
	var err error

	yh.YDLS, err = ydls.NewFromFile(*formatsFlag)
	if err != nil {
		log.Fatalf("failed to read formats: %s", err)
	}
	if *infoFlag {
		yh.InfoLog = log.New(os.Stdout, "INFO: ", log.Ltime)
	}
	if *debugFlag {
		yh.DebugLog = log.New(os.Stdout, "DEBUG: ", log.Ltime)
	}
	if *indexFlag != "" {
		yh.IndexTmpl, err = template.ParseFiles(*indexFlag)
		if err != nil {
			log.Fatalf("failed to parse index template: %s", err)
		}
	}

	log.Printf("Service listen on %s", *listenFlag)
	log.Fatal(http.ListenAndServe(*listenFlag, yh))
}
