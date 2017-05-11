// https://raw.githubusercontent.com/minimaxir/big-list-of-naughty-strings/master/blns.json
// Usage: go run blns.go -p 100 -f blns.json echo 'testing $1'

package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var blnsFileFlag = flag.String("f", "blns.json", "blns.json file")
var parallelismFlag = flag.Int("p", 1, "parallelism")

func main() {
	flag.Parse()

	blnsFile, blnsFileErr := os.Open(*blnsFileFlag)
	if blnsFileErr != nil {
		log.Fatal(blnsFileErr)
	}
	defer blnsFile.Close()

	blns := []string{}
	blnsDecoder := json.NewDecoder(blnsFile)
	blnsDecoderErr := blnsDecoder.Decode(&blns)
	if blnsDecoderErr != nil {
		log.Fatal(blnsDecoderErr)
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	type job struct {
		index int
		s     string
		err   error
	}
	workqueue := make(chan job)
	logqueue := make(chan job)

	worker := func() {
		for s := range workqueue {
			args := []string{}
			for _, a := range flag.Args() {
				args = append(args, strings.Replace(a, "$1", s.s, -1))
			}

			c := exec.CommandContext(ctx, args[0], args...)
			output, err := c.CombinedOutput()

			logqueue <- job{
				index: s.index,
				s:     string(output),
				err:   err,
			}
		}
	}

	logger := func() {
		for s := range logqueue {
			log.Printf("%d ================================", s.index)
			log.Printf("Error: %v", s.err)
			log.Printf("Output:\n%v", s.s)
		}
	}

	loggerWG := sync.WaitGroup{}
	go func() {
		loggerWG.Add(1)
		logger()
		loggerWG.Done()
	}()

	workerWG := sync.WaitGroup{}
	for i := 0; i < *parallelismFlag; i++ {
		go func() {
			workerWG.Add(1)
			worker()
			workerWG.Done()
		}()
	}

	for index, s := range blns {
		workqueue <- job{
			index: index,
			s:     s,
		}
	}

	close(workqueue)
	workerWG.Wait()
	close(logqueue)
	loggerWG.Wait()
}
