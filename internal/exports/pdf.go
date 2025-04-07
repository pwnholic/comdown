package exports

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/pwnholic/comdown/internal"
	"github.com/signintech/gopdf"
)

const (
	defaultJPEGQuality = 90
	dpi                = 128
	ptsPerInch         = 72
	maxImageSize       = 5000
	maxMegapixels      = 25
)

type PDFGenerator struct {
	pdf   *gopdf.GoPdf
	mutex sync.Mutex
}

func NewPDFGenerator() *PDFGenerator {
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{
		Unit:     gopdf.UnitPT,
		PageSize: *gopdf.PageSizeA4,
	})
	return &PDFGenerator{pdf: pdf}
}

func (p *PDFGenerator) AddImageToPDF(imgBytes []byte, fileName, rawURL string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(imgBytes) == 0 {
		internal.WarningLog("Skipping empty image data\n")
		return nil
	}

	img, format, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		internal.WarningLog("Failed to decode image: %v\n", err)
		return nil
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	validFormats := []string{"jpg", "jpeg", "webp"}
	if !slices.Contains(validFormats, strings.ToLower(format)) {
		internal.WarningLog("Skipping unsupported image format: %s (only jpg/jpeg/webp allowed)\n", format)
		return nil
	}

	if err := p.addImageToPage(imgBytes, width, height); err != nil {
		internal.WarningLog("%v\n", err)
		return nil
	}

	logImageInfo(format, width, height, rawURL, fileName)
	return nil
}

func (p *PDFGenerator) addImageToPage(imgBytes []byte, width, height int) error {
	imageHolder, err := gopdf.ImageHolderByBytes(imgBytes)
	if err != nil {
		return fmt.Errorf("failed to create PDF image holder: %w", err)
	}

	pageSize := &gopdf.Rect{
		W: float64(width)*ptsPerInch/dpi - 1,
		H: float64(height)*ptsPerInch/dpi - 1,
	}

	p.pdf.AddPageWithOption(gopdf.PageOption{PageSize: pageSize})
	if err := p.pdf.ImageByHolder(imageHolder, 0, 0, nil); err != nil {
		return fmt.Errorf("failed to add image to PDF: %w", err)
	}

	return nil
}

func logImageInfo(format string, width, height int, rawURL, fileName string) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		internal.WarningLog("invalid URL: %v\n", err)
		return
	}
	lastSegment := path.Base(parsedURL.Path)
	internal.InfoLog("format: (%s), size: (%dx%d), image: (%s) pdf: (%s)\n",
		format, width, height, lastSegment, fileName)
}

func (p *PDFGenerator) SavePDF(outputPath string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.pdf == nil {
		return errors.New("PDF not initialized")
	}
	return p.pdf.WritePdf(outputPath)
}

func (p *PDFGenerator) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.pdf != nil {
		p.pdf.Close()
		p.pdf = nil
	}
}

func (p *PDFGenerator) Reset() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	newPdf := &gopdf.GoPdf{}
	newPdf.Start(gopdf.Config{
		Unit:     gopdf.UnitPT,
		PageSize: *gopdf.PageSizeA4,
	})
	newPdf.SetCompressLevel(0)
	p.pdf = newPdf
}
