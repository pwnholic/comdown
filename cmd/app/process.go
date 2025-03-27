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
}

func NewGenerateProcess(t *clients.HTTPClientOptions) *generateProcess {
	return &generateProcess{
		clients:  *clients.NewRequestBuilder(t),
		exporter: *exports.NewDocumentExporter(),
	}
}

func (gp *generateProcess) processChapters(flag *Flag, comicDir string) {
	startTime := time.Now()
	internal.InfoLog("Starting chapter processing with %d max workers\n", flag.MaxConcurrent)

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(flag.MaxConcurrent)

	attr := gp.clients.Website.GetHTMLTagAttrFromURL(flag.RawURL)
	if attr == nil {
		internal.ErrorLog("HTML attribute not found or Website unsupported")
		os.Exit(1)
	}

	comicMeta := clients.ComicMetadata{
		MaxChapter:    flag.MaxChapter,
		MinChapter:    flag.MaxChapter,
		RawURL:        flag.RawURL,
		Single:        flag.Single,
		ScraperConfig: *attr,
	}

	allLinks, err := gp.clients.Request.CollectLinks(&comicMeta)
	if err != nil {
		internal.ErrorLog("Error fetching links: %v\n", err)
		os.Exit(1)
	}

	var (
		mu             sync.Mutex
		generatedFiles []string
		fileCache      sync.Map
		batchLink      []map[float64][]string
		totalImages    int
	)

	internal.InfoLog("Processing %d chapters\n", len(allLinks))

	isFileExists := func(filename string, cache *sync.Map) bool {
		if val, ok := cache.Load(filename); ok {
			return val.(bool)
		}
		info, err := os.Stat(filename)
		exists := err == nil && !info.IsDir()
		cache.Store(filename, exists)
		return exists
	}

	for _, url := range allLinks {
		rawURL := url
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				titleStr := gp.clients.Website.GetChapterNumber(rawURL)
				if titleStr == "" {
					internal.ErrorLog("Could not extract chapter number from URL: %s\n", rawURL)
					return fmt.Errorf("could not extract chapter number")
				}

				outputFilename := filepath.Join(comicDir, fmt.Sprintf("%s.pdf", titleStr))

				if isFileExists(outputFilename, &fileCache) {
					internal.InfoLog("File already exists, skipping: %s\n", outputFilename)
					return nil
				}

				comicMeta := clients.ComicMetadata{
					RawURL:        rawURL,
					ScraperConfig: *attr,
				}

				imgFromPage, err := gp.clients.Request.CollectImgTagsLink(&comicMeta)
				if err != nil || len(imgFromPage) == 0 {
					internal.ErrorLog("Error fetching page links: %v\n", err)
					return err
				}

				mu.Lock()
				totalImages += len(imgFromPage)
				mu.Unlock()

				if flag.BatchSize > 0 {
					titleFloat, err := strconv.ParseFloat(titleStr, 64)
					if err != nil {
						internal.WarningLog("Chapter title is not a number, using position instead: %s\n", titleStr)
						titleFloat = float64(len(batchLink) + 1) // Use position as fallback
					}
					mu.Lock()
					batchLink = append(batchLink, map[float64][]string{
						titleFloat: imgFromPage,
					})
					mu.Unlock()
					return nil
				}
				return gp.processChapterImages(imgFromPage, outputFilename, &generatedFiles, &mu, flag)
			}
		})
	}

	if err := g.Wait(); err != nil {
		internal.ErrorLog("Error processing chapters: %v\n", err)
		os.Exit(1)
	}

	if flag.BatchSize > 0 {
		gp.processBatches(batchLink, comicDir, flag, &generatedFiles, &mu)
	}

	internal.InfoLog("[SUMMARY] Processed %d chapters in %v\n", len(allLinks), time.Since(startTime))
	internal.InfoLog("[SUMMARY] Generated %d PDF files\n", len(generatedFiles))
	internal.InfoLog("[SUMMARY] Processed %d images in total\n", totalImages)
}

func (gp *generateProcess) processChapterImages(imgFromPage []string, outputFilename string, generatedFiles *[]string, mu *sync.Mutex, flag *Flag) error {
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

		imageData, err := gp.clients.Request.CollectImage(imgURL, *ext, flag.EnhanceImage)
		if err != nil {
			internal.ErrorLog("Error fetching image: %v\n", err.Error())
			continue
		}

		if err := gp.exporter.PDF.AddImageToPDF(imageData); err != nil {
			internal.ErrorLog("Error adding image to PDF: %s\n", err.Error())
			continue
		}
	}

	if err := gp.exporter.PDF.SavePDF(outputFilename); err != nil {
		internal.ErrorLog("Error saving PDF: %v\n", err)
		return err
	}

	internal.SuccessLog("Saved to %s\n", outputFilename)

	mu.Lock()
	*generatedFiles = append(*generatedFiles, outputFilename)
	mu.Unlock()
	return nil
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

func (gp *generateProcess) processBatches(batchLink []map[float64][]string, comicDir string, flag *Flag, generatedFiles *[]string, mu *sync.Mutex) {
	internal.InfoLog("Starting batch processing with size %d\n", flag.BatchSize)
	batchSize := len(batchLink) / flag.BatchSize
	batches := iterateMapInBatch(batchLink, batchSize)

	batchGroup, ctx := errgroup.WithContext(context.Background())
	batchGroup.SetLimit(flag.MaxConcurrent)

	for _, batch := range batches {
		for title, items := range batch {
			title := title
			items := items
			batchGroup.Go(func() error {
				select {
				case <-ctx.Done():
					internal.ErrorLog("Context cancelled for chapter: %s\n", title)
					return ctx.Err()
				default:
					return gp.processChapterImages(items, filepath.Join(comicDir, fmt.Sprintf("%s.pdf", title)), generatedFiles, mu, flag)
				}
			})
		}
	}
	if err := batchGroup.Wait(); err != nil {
		internal.ErrorLog("Error processing batches: %v\n", err)
		os.Exit(1)
	}
}
