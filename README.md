# Usage

**Comic Downloader - Download manga chapters from supported websites**

Usage: `comdown -url <url>` or `comdown -batch <file>`

```
  -b string
    	Path to file containing multiple URLs (one per line)
  -e	[SLOW OPERATION] Enable image quality enhancement (improves resolution and sharpness)
  -h	Display this help message and exit
  -help
    	Alias for -h
  -m int
    	Merge every N chapters into single PDF (0 = no merging). Example: "5" will combine chapters 1-5, 6-10, etc
  -max int
    	[Range Mode] Starting chapter number (inclusive). Use with max-ch to define a range. Ignored when single is set
  -min int
    	[Range Mode] Ending chapter number (inclusive). Use with min-ch to define a range. Ignored when single is set
  -s int
    	[Single Mode] Download specific chapter number. Takes precedence over range mode if both are set
  -u string
    	Target website URL (e.g. "https://komikindo.id/one-piece")
  -x int
    	Maximum active goroutine (default: 10). Higher values may get rate-limited (default 16)
```

Examples:

- Download single chapter: -url <URL> -single 42 -enhance
- Download range with enhancement: -url <URL> -min-ch 10 -max-ch 20 -enhance
- Batch output without enhancement: -url <URL> -min-ch 1 -max-ch 50 -merge 10
- Process multiple URLs from file: -batch urls.txt -min-ch 1 -max-ch 10

# Website Support

You can add new one by your self or see this [See this](./config.json)
