package exports

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"net/url"
	"path"
	"sync"

	"github.com/pwnholic/comdown/internal"
	"github.com/signintech/gopdf"
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
		internal.WarningLog("Skipping empty image data")
		return nil
	}

	img, format, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		internal.WarningLog("Failed to decode image: %v", err)
		return nil
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	if width < 1 || height < 1 {
		internal.WarningLog("Skipping invalid image dimensions: %dx%d", width, height)
		return nil
	}

	if isSuspiciousBlankImage(img) {
		internal.WarningLog("Skipping suspicious blank/white image with unusual size: %dx%d", width, height)
		return nil
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		internal.WarningLog("Failed to convert image to JPEG: %v (Original format: %s)", err, format)
		return nil
	}

	imageHolder, err := gopdf.ImageHolderByBytes(buf.Bytes())
	if err != nil {
		internal.WarningLog("Failed to create PDF image holder: %v", err)
		return nil
	}

	pageSize := &gopdf.Rect{
		W: float64(width)*72/128 - 1,
		H: float64(height)*72/128 - 1,
	}

	p.pdf.AddPageWithOption(gopdf.PageOption{PageSize: pageSize})
	if err := p.pdf.ImageByHolder(imageHolder, 0, 0, nil); err != nil {
		internal.WarningLog("Failed to add image to PDF: %v", err)
		return nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		internal.WarningLog("invalid URL : %v", err)
		return nil
	}
	lastSegment := path.Base(parsedURL.Path)
	internal.InfoLog("format: (%s), size: (%dx%d), filename: (%s) pdf: (%s) \n", format, width, height, lastSegment, fileName)
	return nil
}

func isSuspiciousBlankImage(img image.Image) bool {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	unusualSize := width > 5000 || height > 5000 || (width*height) > 25000000 // 25MP

	if !unusualSize {
		return false
	}

	samplePoints := []image.Point{
		{bounds.Min.X, bounds.Min.Y},
		{bounds.Max.X - 1, bounds.Min.Y},
		{bounds.Min.X, bounds.Max.Y - 1},
		{bounds.Max.X - 1, bounds.Max.Y - 1},
		{width / 2, height / 2},
	}

	for _, pt := range samplePoints {
		if !isWhitePixel(img, pt.X, pt.Y) {
			return false
		}
	}

	return true
}

func isWhitePixel(img image.Image, x, y int) bool {
	if !image.Pt(x, y).In(img.Bounds()) {
		return false
	}

	r, g, b, a := img.At(x, y).RGBA()
	return r == 0xffff && g == 0xffff && b == 0xffff && a == 0xffff
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
