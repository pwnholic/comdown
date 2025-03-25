# Usage

Komik Downloader - Download manga chapters from supported websites

Build : `go build -mod=vendor -o comdown cmd/*.go`

Usage: `comdown -url <url>`

```
  -batch int
    	Merge every N chapters into single PDF (0 = no merging). Example: "5" will combine chapters 1-5, 6-10, etc
  -enhance
    	[SLOW OPERATION] Enable image quality enhancement (improves resolution and sharpness)
  -h	Display this help message and exit
  -help
    	Alias for -h
  -max-ch int
    	[Range Mode] Ending chapter number (inclusive). Use with min-ch to define a range. Ignored when single is set
  -min-ch int
    	[Range Mode] Starting chapter number (inclusive). Use with max-ch to define a range. Ignored when single is set
  -single int
    	[Single Mode] Download specific chapter number. Takes precedence over range mode if both are set
  -url string
    	Target website URL (e.g. "https://komikindo.id/one-piece-list")
  -x int
    	Maximum active goroutine (default: 10). Higher values may get rate-limited (default 10)
```

Examples:

- Download single chapter: ` -url <URL> -single 42 -enhance`
- Download range with enhancement: `-url <URL> -min-ch 10 -max-ch 20 -enhance`
- Batch output without enhancement: ` -url <URL> -min-ch 1 -max-ch 50 -batch 10`

# Website Support

You can add new one by your self or see this [See this](./config.json)
