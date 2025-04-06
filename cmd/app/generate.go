package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
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

type generateComic struct {
	clients   clients.RequestBuilder
	exporter  exports.DocumentExporter
	flag      *Flag
	pdfPool   sync.Pool
	mutex     sync.Mutex
	fileCache sync.Map
	ctx       context.Context
}

func NewGenerateComic(httpOpts *clients.HTTPClientOptions, flag *Flag) *generateComic {
	return &generateComic{
		clients:  *clients.NewRequestBuilder(httpOpts),
		exporter: *exports.NewDocumentExporter(),
		flag:     flag,
		ctx:      context.Background(),
		pdfPool: sync.Pool{
			New: func() any {
				return exports.NewPDFGenerator()
			},
		},
	}
}

func (gc *generateComic) processGenerateComic() error {
	if len(gc.flag.URLs) < 1 {
		return gc.processSingleComic(gc.flag)
	}
	return gc.processBatchComic()
}

func (gc *generateComic) processBatchComic() error {
	g, ctx := errgroup.WithContext(gc.ctx)
	g.SetLimit(len(gc.flag.URLs))
	errChan := make(chan error, len(gc.flag.URLs))
	for _, url := range gc.flag.URLs {
		g.Go(func(url string) func() error {
			return func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					localFlag := &Flag{
						URL:           url,
						MaxChapter:    gc.flag.MaxChapter,
						MinChapter:    gc.flag.MinChapter,
						Single:        gc.flag.Single,
						MaxConcurrent: gc.flag.MaxConcurrent,
						MergeSize:     gc.flag.MergeSize,
					}
					if err := gc.processSingleComic(localFlag); err != nil {
						errChan <- fmt.Errorf("error processing %s: %w", url, err)
						return err
					}
					return nil
				}
			}
		}(url))
	}

	if err := g.Wait(); err != nil {
		close(errChan)
		var errs []error
		for e := range errChan {
			errs = append(errs, e)
		}
		if len(errs) > 0 {
			return fmt.Errorf("completed with %d errors: %v", len(errs), errors.Join(errs...))
		}
		return err
	}
	return nil
}

func getLastPathSegment(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}
	fullPath := u.Path
	fullPath = strings.TrimSuffix(fullPath, "/")
	lastSegment := path.Base(fullPath)
	return lastSegment, nil
}

func (gc *generateComic) processSingleComic(flag *Flag) error {
	startTime := time.Now()
	internal.InfoLog("Starting chapter processing with %d max workers\n", flag.MaxConcurrent)

	folderName, err := getLastPathSegment(flag.URL)
	if err != nil {
		internal.ErrorLog("Could not get path segment with error: %s", err.Error())
		return err
	}

	dir := fmt.Sprintf("comics/%s", folderName)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create comic directory: %w", err)
	}

	internal.InfoLog("Creating New Directory [%s]\n", dir)
	attr := gc.clients.Website.GetHTMLTagAttrFromURL(flag.URL)
	if attr == nil {
		return fmt.Errorf("HTML attribute not found or website unsupported")
	}

	comicMeta := clients.ComicMetadata{
		MaxChapter:    flag.MaxChapter,
		MinChapter:    flag.MinChapter,
		URL:           flag.URL,
		Single:        flag.Single,
		ScraperConfig: *attr,
	}

	allLinks, err := gc.clients.Request.CollectLinks(&comicMeta)
	if err != nil {
		return fmt.Errorf("error fetching links: %w", err)
	}

	internal.InfoLog("Processing %d chapters\n", len(allLinks))
	results, err := gc.processChapterLinks(dir, allLinks, attr)
	if err != nil {
		return err
	}

	if gc.flag.MergeSize > 0 {
		if err := gc.processMergeChapter(results.batchLinks, folderName); err != nil {
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

func (gc *generateComic) processChapterLinks(comicDir string, allLinks []string, attr *clients.ScraperConfig) (*processResults, error) {
	g, ctx := errgroup.WithContext(gc.ctx)
	g.SetLimit(gc.flag.MaxConcurrent)

	var (
		generatedFiles []string
		batchLinks     []map[float64][]string
		totalImages    int
	)

	for _, rawURL := range allLinks {
		rawURL := rawURL
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return errors.Join(ctx.Err(), fmt.Errorf("for this link %s", rawURL))
			default:
				return gc.processComicChapter(
					comicDir,
					rawURL,
					attr,
					&generatedFiles,
					&batchLinks,
					&totalImages,
				)
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

func (gc *generateComic) processComicChapter(
	comicDir,
	rawURL string,
	attr *clients.ScraperConfig,
	generatedFiles *[]string,
	batchLinks *[]map[float64][]string,
	totalImages *int,
) error {
	titleStr, err := gc.clients.Website.GetChapterNumber(rawURL)
	if err != nil {
		internal.ErrorLog("could not extract chapter number from URL: %s", rawURL)
		return err
	}

	outputFilename := filepath.Join(comicDir, fmt.Sprintf("%s.pdf", titleStr))

	if isFileExists(outputFilename, &gc.fileCache) {
		internal.InfoLog("File already exists, skipping: %s\n", outputFilename)
		return nil
	}

	comicMeta := clients.ComicMetadata{
		URL:           rawURL,
		ScraperConfig: *attr,
	}

	imgFromPage, err := gc.clients.Request.CollectImgTagsLink(&comicMeta)
	if err != nil {
		return fmt.Errorf("error fetching page links: %w", err)
	}
	if len(imgFromPage) == 0 {
		return fmt.Errorf("no images found for chapter: %s", rawURL)
	}

	gc.mutex.Lock()
	*totalImages += len(imgFromPage)
	gc.mutex.Unlock()

	if gc.flag.MergeSize > 0 {
		titleFloat, err := strconv.ParseFloat(titleStr, 64)
		if err != nil {
			internal.WarningLog("Chapter title is not a number, using position instead: %s\n", titleStr)
			titleFloat = float64(len(*batchLinks) + 1)
		}
		gc.mutex.Lock()
		*batchLinks = append(*batchLinks, map[float64][]string{
			titleFloat: imgFromPage,
		})
		gc.mutex.Unlock()
		return nil
	}

	return gc.processChapterImages(imgFromPage, outputFilename, generatedFiles)
}

func (gc *generateComic) processChapterImages(imgFromPage []string, outputFilename string, generatedFiles *[]string) error {
	pdfGen := gc.pdfPool.Get().(*exports.PDFGenerator)
	if len(imgFromPage) < 1 {
		internal.ErrorLog("image form page should not be 0")
		gc.pdfPool.Put(pdfGen)
		return nil
	}

	for _, imgURL := range imgFromPage {
		lowerCaseImgURL := strings.ToLower(imgURL)
		ext := gc.clients.Website.GetImageExtension(lowerCaseImgURL)
		if ext == "" {
			internal.WarningLog("Unsupported image format: %s\n", lowerCaseImgURL)
			continue
		}

		if strings.Contains(ext, "gif") {
			internal.WarningLog("Skipping gif %s\n", imgURL)
			continue
		}

		imageData, err := gc.clients.Request.CollectImage(lowerCaseImgURL, ext, gc.flag.EnhanceImage)
		if imageData == nil {
			internal.ErrorLog("This link [%s] has empty image", lowerCaseImgURL)
			return nil
		}

		if err != nil {
			internal.ErrorLog("could not get image byte data with error :%s", err.Error())
			return err
		}

		if err := pdfGen.AddImageToPDF(imageData); err != nil {
			internal.ErrorLog("Error adding image to PDF for this [%s] link with error [%s] \n", lowerCaseImgURL, err.Error())
			return err
		}
	}

	if err := pdfGen.SavePDF(outputFilename); err != nil {
		gc.pdfPool.Put(pdfGen) // <<-- pastikan dikembalikan meski error
		return err
	}

	gc.pdfPool.Put(pdfGen)

	internal.SuccessLog("Saved to %s\n", outputFilename)

	gc.mutex.Lock()
	*generatedFiles = append(*generatedFiles, outputFilename)
	gc.mutex.Unlock()
	return nil
}

func (gc *generateComic) processMergeChapter(batchLinks []map[float64][]string, comicDir string) error {
	internal.InfoLog("Starting batch processing with size %d\n", gc.flag.MergeSize)
	batchSize := len(batchLinks) / gc.flag.MergeSize
	if batchSize <= 0 {
		batchSize = 1
	}
	batches := iterateMapInBatch(batchLinks, batchSize)

	var generatedFiles []string

	g, ctx := errgroup.WithContext(gc.ctx)
	g.SetLimit(gc.flag.MaxConcurrent)

	for _, batch := range batches {
		for title, items := range batch {
			title, items := title, items
			g.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					filename := filepath.Join(comicDir, fmt.Sprintf("%s.pdf", title))
					err := gc.processChapterImages(items, filename, &generatedFiles)
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
