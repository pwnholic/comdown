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

	customFlag := parseFlag()

	startTime := time.Now()

	t := &clients.HTTPClientOptions{
		RetryCount:       5,
		RetryWaitTime:    5,
		RetryMaxWaitTime: 5,
		Timeout:          10,
		UserAgent:        clients.UserAgents[rand.Intn(len(clients.UserAgents))], // mostly website will block our request so i do this :))
	}

	process := NewGenerateProcess(t)

	comicDir := "comic"
	internal.InfoLog("Creating comic directory: %s\n", comicDir)
	if err := os.MkdirAll(comicDir, os.ModePerm); err != nil {
		internal.ErrorLog("Failed creating comic directory: %v\n", err)
		os.Exit(1)
	}

	process.processChapters(customFlag, comicDir)
	internal.SuccessLog("Program completed in %v\n", time.Since(startTime))
}
