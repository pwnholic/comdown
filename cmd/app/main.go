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

	userAgent := clients.UserAgents[rand.Intn(len(clients.UserAgents))] // mostly website will block our request so i do this :))
	t := &clients.HTTPClientOptions{
		RetryCount:       5,
		RetryWaitTime:    5,
		RetryMaxWaitTime: 5,
		Timeout:          10,
		UserAgent:        userAgent,
	}

	process := NewGenerateProcess(t)
	process.processGenerateComic(customFlag)
	internal.SuccessLog("Program completed in %v\n", time.Since(startTime))
}
