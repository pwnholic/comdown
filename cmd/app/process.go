package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/pwnholic/comdown/internal"
	"github.com/pwnholic/comdown/internal/clients"
	"github.com/pwnholic/comdown/internal/exports"
)

type generateProcess struct {
	clients  clients.RequestBuilder
	exporter exports.DocumentExporter
	pdfPool  sync.Pool
}

func NewGenerateProcess(t *clients.HTTPClientOptions) *generateProcess {
	return &generateProcess{
		clients:  *clients.NewRequestBuilder(t),
		exporter: *exports.NewDocumentExporter(),
		pdfPool: sync.Pool{
			New: func() any {
				return exports.NewPDFGenerator()
			},
		},
	}
}

func (gp *generateProcess) processChapters(flag *Flag, comicDir string) error {
	startTime := time.Now()
	internal.InfoLog("Starting chapter processing with %d max workers\n", flag.MaxConcurrent)

	if err := os.MkdirAll(comicDir, 0755); err != nil {
		return fmt.Errorf("failed to create comic directory: %w", err)
	}

	attr := gp.clients.Website.GetHTMLTagAttrFromURL(flag.RawURL)
	if attr == nil {
		return fmt.Errorf("HTML attribute not found or website unsupported")
	}

	comicMeta := clients.ComicMetadata{
		MaxChapter:    flag.MaxChapter,
		MinChapter:    flag.MinChapter,
		RawURL:        flag.RawURL,
		Single:        flag.Single,
		ScraperConfig: *attr,
	}

	allLinks, err := gp.clients.Request.CollectLinks(&comicMeta)
	if err != nil {
		return fmt.Errorf("error fetching links: %w", err)
	}

	internal.InfoLog("Processing %d chapters\n", len(allLinks))

	results, err := gp.processChapterLinks(flag, comicDir, allLinks, attr)
	if err != nil {
		return err
	}

	if flag.BatchSize > 0 {
		if err := gp.processChapterBatches(results.batchLinks, flag, comicDir); err != nil {
			return err
		}
	}

	internal.InfoLog("[SUMMARY] Processed %d chapters in %v\n", len(allLinks), time.Since(startTime))
	internal.InfoLog("[SUMMARY] Generated %d PDF files\n", len(results.generatedFiles))
	internal.InfoLog("[SUMMARY] Processed %d images in total\n", results.totalImages)

	return nil
}

type processResults struct {
	generatedFiles []string
	batchLinks     []map[float64][]string
	totalImages    int
}

func (gp *generateProcess) processChapterLinks(flag *Flag, comicDir string, allLinks []string, attr *clients.ScraperConfig) (*processResults, error) {
	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(flag.MaxConcurrent)

	var (
		mu             sync.Mutex
		generatedFiles []string
		fileCache      sync.Map
		batchLinks     []map[float64][]string
		totalImages    int
	)

	for _, rawURL := range allLinks {
		rawURL := rawURL

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return gp.processSingleChapter(flag, comicDir, rawURL, attr, &mu, &generatedFiles, &fileCache, &batchLinks, &totalImages)
			}
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("error processing chapters: %w", err)
	}

	return &processResults{
		generatedFiles: generatedFiles,
		batchLinks:     batchLinks,
		totalImages:    totalImages,
	}, nil
}

func (gp *generateProcess) processSingleChapter(flag *Flag, comicDir, rawURL string, attr *clients.ScraperConfig,
	mu *sync.Mutex, generatedFiles *[]string, fileCache *sync.Map, batchLinks *[]map[float64][]string, totalImages *int,
) error {
	titleStr := gp.clients.Website.GetChapterNumber(rawURL)
	if titleStr == "" {
		return fmt.Errorf("could not extract chapter number from URL: %s", rawURL)
	}

	outputFilename := filepath.Join(comicDir, fmt.Sprintf("%s.pdf", titleStr))

	if isFileExists(outputFilename, fileCache) {
		internal.InfoLog("File already exists, skipping: %s\n", outputFilename)
		return nil
	}

	comicMeta := clients.ComicMetadata{
		RawURL:        rawURL,
		ScraperConfig: *attr,
	}

	imgFromPage, err := gp.clients.Request.CollectImgTagsLink(&comicMeta)
	if err != nil {
		return fmt.Errorf("error fetching page links: %w", err)
	}
	if len(imgFromPage) == 0 {
		return fmt.Errorf("no images found for chapter: %s", rawURL)
	}

	mu.Lock()
	*totalImages += len(imgFromPage)
	mu.Unlock()

	if flag.BatchSize > 0 {
		titleFloat, err := strconv.ParseFloat(titleStr, 64)
		if err != nil {
			internal.WarningLog("Chapter title is not a number, using position instead: %s\n", titleStr)
			titleFloat = float64(len(*batchLinks) + 1)
		}
		mu.Lock()
		*batchLinks = append(*batchLinks, map[float64][]string{
			titleFloat: imgFromPage,
		})
		mu.Unlock()
		return nil
	}

	return gp.processChapterImages(imgFromPage, outputFilename, generatedFiles, mu, flag)
}

func (gp *generateProcess) processChapterImages(
	imgFromPage []string, outputFilename string, generatedFiles *[]string, mu *sync.Mutex, flag *Flag,
) error {
	// Get PDF generator from pool
	pdfGen := gp.pdfPool.Get().(*exports.PDFGenerator)
	defer func() {
		pdfGen.Reset()
		gp.pdfPool.Put(pdfGen)
	}()

	for _, imgURL := range imgFromPage {
		lowerCaseImgURL := strings.ToLower(imgURL)

		ext := gp.clients.Website.GetImageExtension(lowerCaseImgURL)
		if ext == nil {
			internal.WarningLog("Unsupported image format: %s\n", lowerCaseImgURL)
			continue
		}

		if strings.Contains(*ext, "gif") {
			internal.WarningLog("Skipping gif %s\n", imgURL)
			continue
		}

		var imageData []byte
		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			imageData, _, err = gp.clients.Request.CollectImage(imgURL, *ext, flag.EnhanceImage)
			if err == nil {
				break
			}
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		if err != nil {
			internal.ErrorLog("Failed to fetch image after 3 attempts: %s\n", imgURL)
			continue
		}

		if err := pdfGen.AddImageToPDF(imageData); err != nil {
			internal.ErrorLog("Error adding image to PDF: %s\n", err.Error())
			continue
		}
	}

	if err := pdfGen.SavePDF(outputFilename); err != nil {
		return fmt.Errorf("error saving PDF: %w", err)
	}

	internal.SuccessLog("Saved to %s\n", outputFilename)

	mu.Lock()
	*generatedFiles = append(*generatedFiles, outputFilename)
	mu.Unlock()
	return nil
}

func (gp *generateProcess) processChapterBatches(batchLinks []map[float64][]string, flag *Flag, comicDir string) error {
	internal.InfoLog("Starting batch processing with size %d\n", flag.BatchSize)
	batchSize := len(batchLinks) / flag.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}
	batches := iterateMapInBatch(batchLinks, batchSize)

	var (
		mu             sync.Mutex
		generatedFiles []string
	)

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(flag.MaxConcurrent)

	for _, batch := range batches {
		for title, items := range batch {
			title, items := title, items
			g.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					err := gp.processChapterImages(items, filepath.Join(comicDir, fmt.Sprintf("%s.pdf", title)), &generatedFiles, &mu, flag)
					if err != nil {
						return fmt.Errorf("error processing batch %s: %w", title, err)
					}
					return nil
				}
			})
		}
	}

	return g.Wait()
}

func isFileExists(filename string, cache *sync.Map) bool {
	if val, ok := cache.Load(filename); ok {
		return val.(bool)
	}
	_, err := os.Stat(filename)
	exists := err == nil
	cache.Store(filename, exists)
	return exists
}

func iterateMapInBatch(data []map[float64][]string, batchSize int) []map[string][]string {
	type chapter struct {
		num    float64
		images []string
	}
	var chapters []chapter

	for _, m := range data {
		for chapterNum, imageLink := range m {
			chapters = append(chapters, chapter{chapterNum, imageLink})
		}
	}

	sort.Slice(chapters, func(i, j int) bool {
		return chapters[i].num < chapters[j].num
	})

	var batches []map[string][]string

	for i := 0; i < len(chapters); i += batchSize {
		end := min(i+batchSize, len(chapters))
		batch := chapters[i:end]

		var images []string
		startNum := batch[0].num
		endNum := batch[len(batch)-1].num

		title := fmt.Sprintf("%g", startNum)
		if startNum != endNum {
			title = fmt.Sprintf("%g-%g", startNum, endNum)
		}

		title = strings.ReplaceAll(title, ".0", "")
		title = strings.ReplaceAll(title, "-.0", "-")

		for _, ch := range batch {
			images = append(images, ch.images...)
		}
		batches = append(batches, map[string][]string{title: images})
	}
	return batches
}
