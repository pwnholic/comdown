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
	help := flag.Bool("h", false, "Display this help message and exit")
	flag.BoolVar(help, "help", false, "Alias for -h")
	url := flag.String("u", "", `Target website URL (e.g. "https://komikindo.id/one-piece")`)
	batchFile := flag.String("b", "", `Path to file containing multiple URLs (one per line)`)
	minChapter := flag.Int("max", 0, `[Range Mode] Starting chapter number (inclusive). Use with max-ch to define a range. Ignored when single is set`)
	maxChapter := flag.Int("min", 0, `[Range Mode] Ending chapter number (inclusive). Use with min-ch to define a range. Ignored when single is set`)
	isSingle := flag.Int("s", 0, `[Single Mode] Download specific chapter number. Takes precedence over range mode if both are set`)
	maxConcurrent := flag.Int("x", 16, `Maximum active goroutine (default: 10). Higher values may get rate-limited`)
	mergeSize := flag.Int("m", 0, `Merge every N chapters into single PDF (0 = no merging). Example: "5" will combine chapters 1-5, 6-10, etc`)
	enhance := flag.Bool("e", false, `[SLOW OPERATION] Enable image quality enhancement (improves resolution and sharpness)`)

	flag.Parse()

	if *help {
		fmt.Println("Comic Downloader - Download manga chapters from supported websites")
		fmt.Println("Usage: `comdown -url <url>` or `comdown -batch <file>`")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  Download single chapter: -url <URL> -single 42 -enhance")
		fmt.Println("  Download range with enhancement: -url <URL> -min-ch 10 -max-ch 20 -enhance")
		fmt.Println("  Batch output without enhancement: -url <URL> -min-ch 1 -max-ch 50 -merge 10")
		fmt.Println("  Process multiple URLs from file: -batch urls.txt -min-ch 1 -max-ch 10")
		os.Exit(0)
	}

	if *url == "" && *batchFile == "" {
		fmt.Println("Either URL or batch file is required. Use -url or -batch flag")
		os.Exit(1)
	}

	if *url != "" && *batchFile != "" {
		internal.ErrorLog("Cannot use both -url and -batch at the same time")
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
		internal.WarningLog("-single takes precedence over range flags")
		os.Exit(1)
	}

	if *minChapter > *maxChapter && *isSingle == 0 {
		internal.ErrorLog("-min-ch must be <= max-ch")
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
