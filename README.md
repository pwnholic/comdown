# Usage

**Comic Downloader - Download manga chapters from supported websites**

Usage: `comdown -url <url>` or `comdown -batch <file>`

```
  -M int
    	Merge every N chapters into one PDF
  -b string
    	File with list of URLs
  -e	Enhance image quality (slower)
  -h	Show help
  -help
    	Alias for -h
  -max int
    	End chapter (for range)
  -min int
    	Start chapter (for range)
  -s int
    	Download specific chapter (overrides range)
  -u string
    	Target URL (e.g. https://komikindo.id/one-piece)
  -x int
    	Max goroutines (default 10) (default 16)
```

Examples:

- Download single chapter: ` -u <URL> -single 42 -e`
- Download range with enhancement: `-u <URL> -min 10 -max 20 -e`
- Batch output without enhancement: `-u <URL> -min 1 -max 50 -M 10`
- Process multiple URLs from file: `-b urls.txt -min 1 -max 10`

# Website Support

You can add new one by your self or see this [See this](./config.json)
