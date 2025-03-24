package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/signintech/gopdf"
	"golang.org/x/image/webp"
	"golang.org/x/net/html/charset"
	"golang.org/x/sync/errgroup"
	"resty.dev/v3"
)

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
		return fmt.Errorf("failed to create image holder: %w", err)
	}

	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(imageData))
	if err != nil {
		return fmt.Errorf("failed to decode image config: %w", err)
	}

	c.pdf.AddPageWithOption(gopdf.PageOption{PageSize: &gopdf.Rect{
		W: float64(imageConfig.Width)*72/128 - 1,
		H: float64(imageConfig.Height)*72/128 - 1,
	}})
	return c.pdf.ImageByHolder(imageHolder, 0, 0, nil)
}

func (c *pdfComicImage) savePDF(outputPath string) error {
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
}

func newClientRequest(t *requestTimeOut) *clientRequest {
	client := resty.New().
		SetRetryCount(t.retryCount).
		SetRetryWaitTime(t.retryWaitTime * time.Second).
		SetRetryMaxWaitTime(t.retryMaxWaitTime * time.Second).
		SetTimeout(t.timeOut * time.Second)

	return &clientRequest{
		client: client,
	}
}

func (d *clientRequest) getAllChapterLinks(opts options, htmlTag string) ([]string, error) {
	isRange := opts.minChapter > 0 && opts.maxChapter >= opts.minChapter
	isSingle := opts.isSingle != 0

	response, err := d.client.R().Get(opts.url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer response.Body.Close()

	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.Body, contentType)
	if err != nil {
		return nil, fmt.Errorf("failed to create charset reader: %w", err)
	}

	document, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML document: %w", err)
	}

	var links []string
	document.Find(htmlTag).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			links = append(links, href)
		}
	})

	// Reverse links
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}

	if isRange && !isSingle {
		return links[opts.minChapter-1 : opts.maxChapter], nil
	} else if isSingle && !isRange {
		return links[opts.isSingle-1 : opts.isSingle], nil
	}
	return links, nil
}

func (d *clientRequest) getLinkFromPage(urlRaw, imgPageTag string) ([]string, error) {
	response, err := d.client.R().Get(urlRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer response.Body.Close()

	contentType := response.Header().Get("Content-Type")
	bodyReader, err := charset.NewReader(response.Body, contentType)
	if err != nil {
		return nil, fmt.Errorf("failed to create charset reader: %w", err)
	}

	document, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML document: %w", err)
	}

	var links []string
	document.Find(imgPageTag).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("src")
		if exists {
			links = append(links, href)
		}
	})
	return links, nil
}

func (c *clientRequest) fetchImage(imgLink, ext string) ([]byte, error) {
	resp, err := c.client.R().Get(imgLink)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}
	defer resp.Body.Close()

	buff := new(bytes.Buffer)
	_, err = buff.ReadFrom(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	contentType := resp.Header().Get("Content-Type")
	if contentType == "image/webp" || ext == "webp" {
		img, err := webp.Decode(buff)
		if err != nil {
			return nil, fmt.Errorf("failed to decode webp image: %w", err)
		}

		outputBuff := new(bytes.Buffer)
		err = jpeg.Encode(outputBuff, img, &jpeg.Options{Quality: 100})
		if err != nil {
			return nil, fmt.Errorf("failed to encode image: %w", err)
		}

		return outputBuff.Bytes(), nil
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

func (c *clientRequest) processChapters(opts *options, comicDir string) {
	g := new(errgroup.Group)
	g.SetLimit(opts.maxProcessing)

	allLink, err := c.getAllChapterLinks(*opts, "ul li span.lchx a")
	if err != nil {
		fmt.Printf("Error fetching links: %v\n", err)
		os.Exit(1)
	}

	var generatedFiles []string
	for al := range slices.Values(allLink) {
		al := al
		g.Go(func() error {
			outputFilename := filepath.Join(comicDir, fmt.Sprintf("%s.pdf", getChapterName(al)))
			imgFromPage, err := c.getLinkFromPage(al, "div#chimg-auh img")
			if err != nil {
				return fmt.Errorf("error fetching page links: %w", err)
			}

			if len(imgFromPage) < 1 {
				return fmt.Errorf("error to get page links: %v", err)
			}

			comicFile := newPDFComicImage()
			for imgURL := range slices.Values(imgFromPage) {
				lowerCaseImgURL := strings.ToLower(imgURL)

				ext, err := getImageExtensionFromURL(lowerCaseImgURL)
				if err != nil {
					return fmt.Errorf("image unsuporrted from this link %s with err : %v", lowerCaseImgURL, err)
				}

				if strings.Contains(ext, "gif") {
					fmt.Printf("WARNING: skipping gif %s\n", imgURL)
					continue
				}

				imageData, err := c.fetchImage(imgURL, ext)
				if err != nil {
					fmt.Printf("Error fetching image: %v\n", err)
					continue
				}

				if err := comicFile.addImage(imageData); err != nil {
					fmt.Printf("Error adding image to PDF: %v\n", err)
					continue
				}
			}

			if err := comicFile.savePDF(outputFilename); err != nil {
				return fmt.Errorf("error saving PDF: %w", err)
			}

			fmt.Printf("saved to %s\n", outputFilename)
			generatedFiles = append(generatedFiles, outputFilename)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		fmt.Printf("Error processing chapters: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	minChapter    int
	maxChapter    int
	url           string
	maxProcessing int
	isSingle      int

	batchSize int // New option for batch size
}

func parseOptions() *options {
	minChapter := flag.Int("min-ch", 0, "Minimum chapter to download (inclusive)")
	isSingle := flag.Int("single", 0, "1 chapter to download")
	maxChapter := flag.Int("max-ch", math.MaxInt, "Maximum chapter to download (inclusive)")
	maxProcessing := flag.Int("x", 10, "Maximum number of concurrent workers")
	url := flag.String("url", "", "Website URL")
	help := flag.Bool("h", false, "Show help message")

	// TODO: this feature not implement yet
	batchSize := flag.Int("batch", 0, "Number of PDFs to merge into one file (0 to disable)")

	flag.BoolVar(help, "help", false, "Show help message") // Alias for -h

	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	if *minChapter > *maxChapter {
		fmt.Println("Error: min-ch must be less than or equal to max-ch")
		flag.Usage()
		os.Exit(1)
	}
	if *minChapter < 0 {
		fmt.Println("Error: min-ch must be greater than or equal to 0")
		flag.Usage()
		os.Exit(1)
	}
	if *maxProcessing <= 0 {
		fmt.Println("Error: maxProcessing must be greater than 0")
		flag.Usage()
		os.Exit(1)
	}
	if *batchSize < 0 {
		fmt.Println("Error: batch size must be greater than or equal to 0")
		flag.Usage()
		os.Exit(1)
	}

	return &options{
		maxChapter:    *maxChapter,
		minChapter:    *minChapter,
		url:           *url,
		maxProcessing: *maxProcessing,
		isSingle:      *isSingle,
		batchSize:     *batchSize,
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
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <url>\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}

	timout := requestTimeOut{
		retryCount:       3,
		retryWaitTime:    2, // second
		retryMaxWaitTime: 5, // second
		timeOut:          5, // second
	}

	opts := parseOptions()
	req := newClientRequest(&timout)
	defer req.client.Close()

	comicDir := "comic"
	if err := os.MkdirAll(comicDir, os.ModePerm); err != nil {
		fmt.Printf("Error creating comic directory: %v\n", err)
		os.Exit(1)
	}

	req.processChapters(opts, comicDir)
}
