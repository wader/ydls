package main

import (
	"flag"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/wader/ydls/ydls"
)

var infoFlag = flag.Bool("info", false, "info output")
var debugFlag = flag.Bool("debug", false, "debug output")
var formatsFlag = flag.String("formats", "formats.json", "formats config file")
var indexFlag = flag.String("index", "", "index html template")
var listenFlag = flag.String("listen", ":8080", "listen address")

var infoLog = log.New(ioutil.Discard, "INFO: ", log.Ltime)
var debugLog = log.New(ioutil.Discard, "DEBUG: ", log.Ltime)

func init() {
	flag.Parse()

	if os.Getenv("DEBUG") != "" {
		*debugFlag = true
	}
	if *infoFlag {
		infoLog.SetOutput(os.Stdout)
	}
	if *debugFlag {
		debugLog.SetOutput(os.Stdout)
	}
}

func main() {
	yh := &ydlsHandler{}
	var err error

	yh.ydls, err = ydls.NewFromFile(*formatsFlag)
	if err != nil {
		log.Fatalf("failed to read formats: %s", err)
	}

	if *indexFlag != "" {
		yh.indexTmpl, err = template.ParseFiles(*indexFlag)
		if err != nil {
			log.Fatalf("failed to parse index template: %s", err)
		}
	}

	log.Printf("Service listen on %s", *listenFlag)
	log.Fatal(http.ListenAndServe(*listenFlag, yh))
}
