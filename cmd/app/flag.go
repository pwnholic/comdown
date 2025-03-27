package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pwnholic/comdown/internal"
)

type Flag struct {
	MaxChapter    int
	MinChapter    int
	RawURL        string
	Single        int
	MaxConcurrent int
	BatchSize     int
	EnhanceImage  bool
}

func parseFlag() *Flag {
	help := flag.Bool("h", false, "Display this help message and exit")
	flag.BoolVar(help, "help", false, "Alias for -h")

	rawURL := flag.String("url", "", `Target website URL (e.g. "https://komikindo.id/one-piece")`)
	minChapter := flag.Int("min-ch", 0, `[Range Mode] Starting chapter number (inclusive). Use with max-ch to define a range. Ignored when single is set`)
	maxChapter := flag.Int("max-ch", 0, `[Range Mode] Ending chapter number (inclusive). Use with min-ch to define a range. Ignored when single is set`)
	isSingle := flag.Int("single", 0, `[Single Mode] Download specific chapter number. Takes precedence over range mode if both are set`)
	maxConcurrent := flag.Int("x", 10, `Maximum active goroutine (default: 10). Higher values may get rate-limited`)
	batchSize := flag.Int("batch", 0, `Merge every N chapters into single PDF (0 = no merging). Example: "5" will combine chapters 1-5, 6-10, etc`)

	// TODO: made this more faster
	enhance := flag.Bool("enhance", false, `[SLOW OPERATION] Enable image quality enhancement (improves resolution and sharpness)`)

	flag.Parse()

	if *help {
		fmt.Println("Comic Downloader - Download manga chapters from supported websites")
		fmt.Println("Usage: `comdown -url <url>`")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  Download single chapter: -url <URL> -single 42 -enhance")
		fmt.Println("  Download range with enhancement: -url <URL> -min-ch 10 -max-ch 20 -enhance")
		fmt.Println("  Batch output without enhancement: -url <URL> -min-ch 1 -max-ch 50 -batch 10")
		os.Exit(0)
	}

	if *rawURL == "" {
		fmt.Println("URL is required. Use -url flag")
		os.Exit(1)
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

	if *batchSize < 0 {
		internal.ErrorLog("Batch size must be >= 0 (0 disables batching)")
		os.Exit(1)
	}

	return &Flag{
		MaxChapter:    *maxChapter,
		MinChapter:    *minChapter,
		RawURL:        *rawURL,
		MaxConcurrent: *maxConcurrent,
		Single:        *isSingle,
		BatchSize:     *batchSize,
		EnhanceImage:  *enhance,
	}
}
