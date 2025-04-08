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

	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0.3 Safari/605.1.15",
		"Mozilla/5.0 (Linux; Android 10; SM-G980F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.120 Mobile Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36",
	}

	userAgent := userAgents[rand.Intn(len(userAgents))]
	httpOpts := &clients.HTTPClientOptions{
		RetryCount:       5,
		RetryWaitTime:    5 * time.Second,
		RetryMaxWaitTime: 5 * time.Second,
		Timeout:          10 * time.Second,
		UserAgent:        userAgent,
	}

	process := NewGenerateComic(httpOpts, customFlag)
	err := process.processGenerateComic()
	if err != nil {
		internal.ErrorLog("Something when wrong : %s", err.Error())
		return
	}
	internal.SuccessLog("Program completed in %v\n", time.Since(startTime))
}
