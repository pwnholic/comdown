package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/pwnholic/comdown/internal"
	"github.com/pwnholic/comdown/internal/clients"
)

func init() {
	internal.InitDefaultLogger(internal.INFO)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: comdown -url <url>")
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}

	startTime := time.Now()
	customFlag := parseFlag()

	// mostly website will block our request so i do this :))
	userAgent := clients.UserAgents[rand.Intn(len(clients.UserAgents))]
	httpOpts := &clients.HTTPClientOptions{
		RetryCount:       5,
		RetryWaitTime:    5,
		RetryMaxWaitTime: 5,
		Timeout:          10,
		UserAgent:        userAgent,
	}

	process := NewGenerateComic(httpOpts, customFlag)
	process.processGenerateComic()
	internal.SuccessLog("Program completed in %v\n", time.Since(startTime))
}
