package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/disintegration/imaging"
	"github.com/signintech/gopdf"
	"golang.org/x/image/webp"
	"golang.org/x/net/html/charset"
	"golang.org/x/sync/errgroup"
	"resty.dev/v3"
)

var logger *log.Logger

func init() {
	logger = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
}

type pdfComicImage struct {
	pdf *gopdf.GoPdf
}

func newPDFComicImage() *pdfComicImage {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{Unit: gopdf.UnitPT, PageSize: *gopdf.PageSizeA4})
	return &pdfComicImage{pdf: &pdf}
}

func (c *pdfComicImage) addImage(imageData []byte) error {
	imageHolder, err := gopdf.ImageHolderByBytes(imageData)
	if err != nil {
		logger.Printf("[ERROR] Failed to create image holder: %v\n", err)
		return fmt.Errorf("failed to create image holder: %w", err)
	}

	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(imageData))
	if err != nil {
		logger.Printf("[ERROR] Failed to decode image config: %v\n", err)
		return fmt.Errorf("failed to decode image config: %w", err)
	}

	c.pdf.AddPageWithOption(gopdf.PageOption{PageSize: &gopdf.Rect{
		W: float64(imageConfig.Width)*72/128 - 1,
		H: float64(imageConfig.Height)*72/128 - 1,
	}})
	return c.pdf.ImageByHolder(imageHolder, 0, 0, nil)
}

func (c *pdfComicImage) savePDF(outputPath string) error {
	logger.Printf("[INFO] Saving PDF to: %s\n", outputPath)
	return c.pdf.WritePdf(outputPath)
}

type clientRequest struct {
	client *resty.Client
}

type requestTimeOut struct {
	retryCount       int
	retryWaitTime    time.Duration
	retryMaxWaitTime time.Duration
	timeOut          time.Duration
	userAgent        string
}

func newClientRequest(t *requestTimeOut) *clientRequest {
	client := resty.New().
		SetRetryCount(t.retryCount).
		SetRetryWaitTime(t.retryWaitTime*time.Second).
		SetRetryMaxWaitTime(t.retryMaxWaitTime*time.Second).
		SetTimeout(t.timeOut*time.Second).
		SetHeader("User-Agent", t.userAgent)

	defer client.Close()

	return &clientRequest{
		client: client,
	}
}

func (c *clientRequest) isStatusCodeOK(resp *resty.Response) (bool, string) {
	switch resp.StatusCode() {
	case http.StatusTooManyRequests:
		return true, "IP blocked: Too Many Requests (429)"
	case http.StatusForbidden:
		return true, "IP blocked: Forbidden (403)"
	case http.StatusServiceUnavailable:
		return true, "IP blocked: Service Unavailable (503)"
	case http.StatusOK:
		return false, "Status OK"
	}
	return false, "IP not blocked"
}

func completeURL(inputURL, defaultHost string) (string, error) {
	if inputURL == "" {
		return "", fmt.Errorf("URL cannot be empty")
	}
	parsedURL, err := url.Parse(inputURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}
	if parsedURL.IsAbs() {
		return inputURL, nil
	}
	if defaultHost == "" {
		return "", fmt.Errorf("host need for relative url: %w", err)
	}
	defaultURL, err := url.Parse(defaultHost)
	if err != nil {
		return "", fmt.Errorf("invalid default host: %v", err)
	}
	if defaultURL.Scheme == "" {
		defaultURL.Scheme = "https"
	}
	resultURL := defaultURL.ResolveReference(parsedURL)
	return resultURL.String(), nil
}

func (d *clientRequest) getAllChapterLinks(opts options, tag htmlTagAttr) ([]string, error) {
	isRange := opts.minChapter > 0 && opts.maxChapter >= opts.minChapter
	isSingle := opts.isSingle != 0

	response, err := d.client.R().Get(opts.urlRaw)
	if err != nil {
		logger.Printf("[ERROR] Failed to fetch URL: %v\n", err)
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer response.Body.Close()

	isIPBlocked, reason := d.isStatusCodeOK(response)
	if isIPBlocked {
		logger.Printf("[WARNING] BLOCKED : %s\n", reason)
	}

	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.Body, contentType)
	if err != nil {
		logger.Printf("[ERROR] Failed to create charset reader: %v\n", err)
		return nil, fmt.Errorf("failed to create charset reader: %w", err)
	}

	document, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		logger.Printf("[ERROR] Failed to parse HTML document: %v\n", err)
		return nil, fmt.Errorf("failed to parse HTML document: %w", err)
	}

	var links []string
	document.Find(tag.listChapterURL).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr(tag.attrChapter)
		if exists {
			result, err := completeURL(href, opts.urlRaw)
			if err != nil {
				logger.Fatalf("[ERROR] Failed to add hostname : %v\n", err)
			}
			links = append(links, result)
		}
	})

	logger.Printf("[DEBUG] Found %d chapter links\n", len(links))

	// Reverse links
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}

	if isRange && !isSingle {
		logger.Printf("[INFO] Filtering chapters range %d-%d\n", opts.minChapter, opts.maxChapter)
		links = links[opts.minChapter-1 : opts.maxChapter]
	} else if isSingle && !isRange {
		logger.Printf("[INFO] Selecting single chapter %d\n", opts.isSingle)
		links = links[opts.isSingle-1 : opts.isSingle]
	}

	return links, nil
}

func (d *clientRequest) getLinkFromPage(rawURL string, tag htmlTagAttr) ([]string, error) {
	response, err := d.client.R().Get(rawURL)
	if err != nil {
		logger.Printf("[ERROR] Failed to fetch URL: %v\n", err)
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer response.Body.Close()

	isIPBlocked, reason := d.isStatusCodeOK(response)
	if isIPBlocked {
		logger.Printf("[WARNING] BLOCKED : %s\n", reason)
	}

	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.Body, contentType)
	if err != nil {
		logger.Printf("[ERROR] Failed to create charset reader: %v\n", err)
		return nil, fmt.Errorf("failed to create charset reader: %w", err)
	}

	document, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		logger.Printf("[ERROR] Failed to parse HTML document: %v\n", err)
		return nil, fmt.Errorf("failed to parse HTML document: %w", err)
	}

	var links []string
	document.Find(tag.listImageURL).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr(tag.attrImage)
		if exists {
			links = append(links, href)
		}
	})
	logger.Printf("[INFO] Found %d images on page %s\n", len(links), rawURL)
	return links, nil
}

func (c *clientRequest) fetchImage(imgLink, ext string, enhance bool) ([]byte, error) {
	resp, err := c.client.R().Get(imgLink)
	if err != nil {
		logger.Printf("[ERROR] Failed to fetch image: %v\n", err)
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}
	defer resp.Body.Close()

	buff := new(bytes.Buffer)
	_, err = buff.ReadFrom(resp.Body)
	if err != nil {
		logger.Printf("[ERROR] Failed to read image data: %v\n", err)
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	contentType := resp.Header().Get("Content-Type")
	enhanceImage := func(imgBytes []byte) ([]byte, error) {
		img, err := imaging.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to decode image for enhancement: %w", err)
		}

		img = imaging.Resize(img, img.Bounds().Dx()*2, img.Bounds().Dy()*2, imaging.Lanczos)
		img = imaging.Sharpen(img, 0.7)
		img = imaging.AdjustContrast(img, 10)

		outBuff := new(bytes.Buffer)
		err = jpeg.Encode(outBuff, img, &jpeg.Options{Quality: 100})
		if err != nil {
			return nil, fmt.Errorf("failed to encode enhanced image: %w", err)
		}

		return outBuff.Bytes(), nil
	}

	if contentType == "image/webp" || ext == "webp" {
		logger.Println("[INFO] Processing WEBP image conversion")
		img, err := webp.Decode(buff)
		if err != nil {
			logger.Printf("[ERROR] Failed to decode webp image: %v\n", err)
			return nil, fmt.Errorf("failed to decode webp image: %w", err)
		}

		outputBuff := new(bytes.Buffer)
		err = jpeg.Encode(outputBuff, img, &jpeg.Options{Quality: 100})
		if err != nil {
			logger.Printf("[ERROR] Failed to encode image: %v\n", err)
			return nil, fmt.Errorf("failed to encode image: %w", err)
		}

		if enhance {
			enhanced, err := enhanceImage(outputBuff.Bytes())
			if err != nil {
				logger.Printf("[WARNING] Failed to enhance image: %v\n", err)
				return outputBuff.Bytes(), nil
			}
			logger.Println("[INFO] WEBP to JPEG conversion with enhancement completed")
			return enhanced, nil
		}

		logger.Println("[INFO] WEBP to JPEG conversion completed")
		return outputBuff.Bytes(), nil
	}

	if enhance {
		enhanced, err := enhanceImage(buff.Bytes())
		if err != nil {
			logger.Printf("[WARNING] Failed to enhance image: %v\n", err)
			return buff.Bytes(), nil
		}
		logger.Println("[INFO] Image enhancement completed")
		return enhanced, nil
	}

	return buff.Bytes(), nil
}

func getImageExtensionFromURL(url string) (string, error) {
	fileName := path.Base(url)
	if fileName == "" {
		return "", fmt.Errorf("invalid URL: no file name found")
	}

	ext := strings.ToLower(path.Ext(fileName))
	if ext == "" {
		return "", fmt.Errorf("no file extension found in URL")
	}

	ext = strings.TrimPrefix(ext, ".")

	supportedExtensions := map[string]bool{
		"jpg":  true,
		"jpeg": true,
		"png":  true,
		"webp": true,
		"gif":  true,
	}
	if !supportedExtensions[ext] {
		return "", fmt.Errorf("unsupported image extension: %s", ext)
	}
	return ext, nil
}

func isFileExists(filename string, cache *sync.Map) bool {
	if val, ok := cache.Load(filename); ok {
		return val.(bool)
	}
	info, err := os.Stat(filename)
	exists := err == nil && !info.IsDir()
	cache.Store(filename, exists)
	return exists
}

func (c *clientRequest) processChapters(opts *options, comicDir string) {
	startTime := time.Now()
	logger.Printf("[INFO] Starting chapter processing with %d max workers\n", opts.maxProcessing)

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(opts.maxProcessing)

	attr := supportedWebsite(opts.urlRaw)
	if attr == nil {
		logger.Println("[ERROR] HTML attribut not found or Website unsupported")
		os.Exit(1)
	}

	allLink, err := c.getAllChapterLinks(*opts, *attr)
	if err != nil {
		logger.Printf("[ERROR] Error fetching links: %v\n", err)
		os.Exit(1)
	}

	var (
		mu             sync.Mutex
		generatedFiles []string
		fileCache      sync.Map
		batchLink      []map[int][]string
	)

	logger.Printf("[INFO] Processing %d chapters\n", len(allLink))

	for _, al := range allLink {
		rawURL := al
		g.Go(func() error {
			select {
			case <-ctx.Done():
				logger.Printf("[WARNING] Context cancelled for chapter: %s\n", rawURL)
				return ctx.Err()
			default:
			}

			titleStr := getChapterName(rawURL)
			outputFilename := filepath.Join(comicDir, fmt.Sprintf("%s.pdf", titleStr))

			if isFileExists(outputFilename, &fileCache) {
				logger.Printf("[INFO] File already exists, skipping: %s\n", outputFilename)
				return nil
			}

			imgFromPage, err := c.getLinkFromPage(rawURL, *attr)
			if err != nil || len(imgFromPage) == 0 {
				logger.Printf("[ERROR] Error fetching page links: %v\n", err)
				return fmt.Errorf("error fetching page links: %w", err)
			}

			if opts.batchSize > 0 {
				titleInt, err := strconv.Atoi(titleStr)
				if err != nil {
					logger.Printf("[ERROR] Could not convert title string to int: %v\n", err)
					return fmt.Errorf("could not convert title string to int: %w", err)
				}
				mu.Lock()
				batchLink = append(batchLink, map[int][]string{
					titleInt: imgFromPage,
				})
				mu.Unlock()
				return nil
			}

			comicFile := newPDFComicImage()
			for _, imgURL := range imgFromPage {
				lowerCaseImgURL := strings.ToLower(imgURL)

				ext, err := getImageExtensionFromURL(lowerCaseImgURL)
				if err != nil {
					logger.Printf("[WARNING] Unsupported image format: %s, err: %v\n", lowerCaseImgURL, err)
					continue
				}

				if strings.Contains(ext, "gif") {
					logger.Printf("[WARNING] Skipping gif %s\n", imgURL)
					continue
				}

				imageData, err := c.fetchImage(imgURL, ext, opts.enhanceImage)
				if err != nil {
					logger.Printf("[ERROR] Error fetching image: %v\n", err)
					continue
				}

				if err := comicFile.addImage(imageData); err != nil {
					logger.Printf("[ERROR] Error adding image to PDF: %v\n", err)
					continue
				}
			}

			if err := comicFile.savePDF(outputFilename); err != nil {
				logger.Printf("[ERROR] Error saving PDF: %v\n", err)
				return fmt.Errorf("error saving PDF: %w", err)
			}

			logger.Printf("[SUCCESS] Saved to %s\n", outputFilename)

			mu.Lock()
			generatedFiles = append(generatedFiles, outputFilename)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		logger.Printf("[ERROR] Error processing chapters: %v\n", err)
		os.Exit(1)
	}

	if opts.batchSize > 0 {
		logger.Printf("[INFO] Starting batch processing with size %d\n", opts.batchSize)
		batchSize := len(allLink) / opts.batchSize
		batches := iterateMapInBatch(batchLink, batchSize)

		batchGroup, ctx := errgroup.WithContext(context.Background())
		batchGroup.SetLimit(opts.maxProcessing)

		for _, batch := range batches {
			for title, items := range batch {
				title := title
				items := items

				batchGroup.Go(func() error {
					select {
					case <-ctx.Done():
						logger.Printf("[WARNING] Context cancelled for batch: %s\n", title)
						return ctx.Err()
					default:
					}

					outputFilename := filepath.Join(comicDir, fmt.Sprintf("%s.pdf", title))
					if isFileExists(outputFilename, &fileCache) {
						logger.Printf("[INFO] Batch file already exists, skipping: %s\n", outputFilename)
						return nil
					}

					comicFile := newPDFComicImage()
					for _, imgURL := range items {
						lowerCaseImgURL := strings.ToLower(imgURL)

						ext, err := getImageExtensionFromURL(lowerCaseImgURL)
						if err != nil {
							logger.Printf("[WARNING] Unsupported image format: %s, err: %v\n", lowerCaseImgURL, err)
							continue
						}

						if strings.Contains(ext, "gif") {
							logger.Printf("[WARNING] Skipping gif %s\n", imgURL)
							continue
						}

						imageData, err := c.fetchImage(imgURL, ext, opts.enhanceImage)
						if err != nil {
							logger.Printf("[ERROR] Error fetching image: %v\n", err)
							continue
						}

						if err := comicFile.addImage(imageData); err != nil {
							logger.Printf("[ERROR] Error adding image to PDF: %v\n", err)
							continue
						}
					}

					if err := comicFile.savePDF(outputFilename); err != nil {
						logger.Printf("[ERROR] Error saving batch PDF: %v\n", err)
						return fmt.Errorf("error saving batch PDF: %w", err)
					}

					logger.Printf("[SUCCESS] Saved batch to %s\n", outputFilename)

					mu.Lock()
					generatedFiles = append(generatedFiles, outputFilename)
					mu.Unlock()
					return nil
				})
			}
		}

		if err := batchGroup.Wait(); err != nil {
			logger.Printf("[ERROR] Error processing batches: %v\n", err)
			os.Exit(1)
		}
	}

	logger.Printf("[SUMMARY] Processed %d chapters in %v\n", len(allLink), time.Since(startTime))
	logger.Printf("[SUMMARY] Generated %d PDF files\n", len(generatedFiles))
}

func iterateMapInBatch(data []map[int][]string, batchSize int) []map[string][]string {
	var result []map[string][]string
	for i := 0; i < len(data); i += batchSize {
		end := min(i+batchSize, len(data))
		var batch []string
		for _, m := range data[i:end] {
			keys := make([]int, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			for _, k := range keys {
				batch = append(batch, m[k]...)
			}
		}
		title := fmt.Sprintf("%d-%d", i, end-1)
		result = append(result, map[string][]string{title: batch})
	}
	return result
}

type options struct {
	minChapter    int
	maxChapter    int
	urlRaw        string
	maxProcessing int
	isSingle      int
	batchSize     int
	enhanceImage  bool
}

func parseOptions() *options {
	help := flag.Bool("h", false, "Display this help message and exit")
	flag.BoolVar(help, "help", false, "Alias for -h")

	urlRaw := flag.String("url", "", `Target website URL (e.g. "https://komikindo.id/one-piece-chapter-1")`)
	minChapter := flag.Int("min-ch", 0, `[Range Mode] Starting chapter number (inclusive). Use with max-ch to define a range. Ignored when single is set`)
	maxChapter := flag.Int("max-ch", 0, `[Range Mode] Ending chapter number (inclusive). Use with min-ch to define a range. Ignored when single is set`)
	isSingle := flag.Int("single", 0, `[Single Mode] Download specific chapter number. Takes precedence over range mode if both are set`)
	maxProcessing := flag.Int("x", 10, `Maximum active goroutine (default: 10). Higher values may get rate-limited`)
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

	if *urlRaw == "" {
		logger.Fatal("[ERROR] URL is required. Use -url flag")
	}

	if *isSingle > 0 && (*minChapter > 0 || *maxChapter > 0) {
		logger.Println("[WARNING] -single takes precedence over range flags")
	}

	if *minChapter > *maxChapter && *isSingle == 0 {
		logger.Fatal("[ERROR] min-ch must be <= max-ch")
	}

	if *maxProcessing < 1 {
		logger.Fatal("[ERROR] Concurrency value (-x) must be >= 1")
	}

	if *batchSize < 0 {
		logger.Fatal("[ERROR] Batch size must be >= 0 (0 disables batching)")
	}

	return &options{
		maxChapter:    *maxChapter,
		minChapter:    *minChapter,
		urlRaw:        *urlRaw,
		maxProcessing: *maxProcessing,
		isSingle:      *isSingle,
		batchSize:     *batchSize,
		enhanceImage:  *enhance,
	}
}

func getChapterName(urlRaw string) string {
	re := regexp.MustCompile(`chapter-(\d+)`)
	matches := re.FindStringSubmatch(urlRaw)
	if len(matches) > 1 {
		return matches[1]
	}
	return urlRaw
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: comdown -url <url>")
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}

	startTime := time.Now()

	timout := requestTimeOut{
		retryCount:       5,
		retryWaitTime:    5,                                      // second
		retryMaxWaitTime: 5,                                      // second
		timeOut:          10,                                     // second
		userAgent:        userAgents[rand.Intn(len(userAgents))], // mostly website will block our request so i do this :))
	}

	opts := parseOptions()
	req := newClientRequest(&timout)

	comicDir := "comic"
	logger.Printf("[INFO] Creating comic directory: %s\n", comicDir)
	if err := os.MkdirAll(comicDir, os.ModePerm); err != nil {
		logger.Printf("[ERROR] Error creating comic directory: %v\n", err)
		os.Exit(1)
	}

	req.processChapters(opts, comicDir)
	logger.Printf("[SUCCESS] Program completed in %v\n", time.Since(startTime))
}
