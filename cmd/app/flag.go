package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pwnholic/comdown/internal"
)

type Flag struct {
	MaxChapter    int
	MinChapter    int
	URL           string
	URLs          []string // New field to store multiple URLs
	Single        int
	MaxConcurrent int
	MergeSize     int
	EnhanceImage  bool
	BatchFile     *string // New field for batch file path
}

func parseFlag() *Flag {
	help := flag.Bool("h", false, "Show help")
	flag.BoolVar(help, "help", false, "Alias for -h")
	url := flag.String("u", "", "Target URL (e.g. https://komikindo.id/one-piece)")
	batchFile := flag.String("b", "", "File with list of URLs")
	minChapter := flag.Int("min", 0, "Start chapter (for range)")
	maxChapter := flag.Int("max", 0, "End chapter (for range)")
	isSingle := flag.Int("s", 0, "Download specific chapter (overrides range)")
	maxConcurrent := flag.Int("x", 16, "Max goroutines (default 10)")
	mergeSize := flag.Int("M", 0, "Merge every N chapters into one PDF")
	enhance := flag.Bool("e", false, "Enhance image quality (slower)")

	flag.Parse()

	if *help {
		fmt.Println("Comic Downloader - Download manga chapters from supported websites")
		fmt.Println("Usage: `comdown -u <url>` or `comdown -b <file>`")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  Download single chapter: -u <URL> -s 42 -e")
		fmt.Println("  Download range with enhancement: -u <URL> -min 10 -max 20 -e")
		fmt.Println("  Batch output without enhancement: -u <URL> -min 1 -max 50 -M 10")
		fmt.Println("  Process multiple URLs from file: -batch urls.txt -min 1 -max 10")
		os.Exit(0)
	}

	if *url == "" && *batchFile == "" {
		fmt.Println("Either URL or batch file is required. Use -u or -b flag")
		os.Exit(1)
	}

	if *url != "" && *batchFile != "" {
		internal.ErrorLog("Cannot use both -u and -b at the same time")
		os.Exit(1)
	}

	// Read URLs from batch file if specified
	var urls []string
	if *batchFile != "" {
		file, err := os.Open(*batchFile)
		if err != nil {
			internal.ErrorLog("Failed to open batch file: %v", err)
			os.Exit(1)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			url := scanner.Text()
			if url != "" {
				log.Println(url)
				urls = append(urls, url)
			}
		}

		if err := scanner.Err(); err != nil {
			internal.ErrorLog("Error reading batch file: %v", err)
			os.Exit(1)
		}

		if len(urls) == 0 {
			internal.ErrorLog("Batch file is empty or contains no valid URLs")
			os.Exit(1)
		}
	}

	if *isSingle > 0 && (*minChapter > 0 || *maxChapter > 0) {
		internal.WarningLog("-s takes precedence over range flags")
		os.Exit(1)
	}

	if *minChapter > *maxChapter && *isSingle == 0 {
		internal.ErrorLog("-min must be <= max")
		os.Exit(1)
	}

	if *maxConcurrent < 1 {
		internal.ErrorLog("Concurrency value (-x) must be >= 1")
		os.Exit(1)
	}

	if *mergeSize < 0 {
		internal.ErrorLog("Merge size must be >= 0 (0 disables batching)")
		os.Exit(1)
	}

	return &Flag{
		MaxChapter:    *maxChapter,
		MinChapter:    *minChapter,
		URL:           *url,
		URLs:          urls,
		MaxConcurrent: *maxConcurrent,
		Single:        *isSingle,
		MergeSize:     *mergeSize,
		EnhanceImage:  *enhance,
		BatchFile:     batchFile,
	}
}
