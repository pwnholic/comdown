package exports

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
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

func isBlankImage(img image.Image) bool {
	bounds := img.Bounds()
	blankPixels := 0
	totalPixels := bounds.Dx() * bounds.Dy()

	// Sample pixels to check for blankness (check every 10th pixel for performance)
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 10 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 10 {
			r, g, b, a := img.At(x, y).RGBA()
			// Consider pixel blank if it's white (or transparent)
			if r == 0xFFFF && g == 0xFFFF && b == 0xFFFF || a == 0 {
				blankPixels++
			}
		}
	}
	// If more than 90% of sampled pixels are blank, consider the image blank
	blankRatio := float64(blankPixels) / float64(totalPixels/100)
	return blankRatio > 90
}

func (p *PDFGenerator) AddImageToPDF(imgBytes []byte) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if imgBytes == nil {
		internal.WarningLog("Skipping empty image data")
		return nil
	}

	sig := bytes.Equal(imgBytes[:2], []byte{0xFF, 0xD8})
	if !sig {
		internal.WarningLog("Invalid JPEG signature, skipping corrupt image")
		return nil
	}

	img, format, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		internal.WarningLog("failed to decode image: %s", err.Error())
		return nil
	}

	// Check for blank/white image
	if isBlankImage(img) {
		internal.WarningLog("Skipping blank/white image")
		return nil
	}

	bounds := img.Bounds()
	if bounds.Dx() < 1 || bounds.Dy() < 1 || format == "" {
		internal.WarningLog("Skipping invalid image | Dimensions: %dx%d | Format: %s",
			bounds.Dx(), bounds.Dy(), format)
		return nil
	}

	// Convert image back to bytes for PDF processing
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return fmt.Errorf("failed to re-encode image: %w", err)
	}

	imageHolder, err := gopdf.ImageHolderByBytes(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to create image holder: %w", err)
	}

	pageSize := &gopdf.Rect{
		W: float64(bounds.Dx())*72/128 - 1,
		H: float64(bounds.Dy())*72/128 - 1,
	}

	p.pdf.AddPageWithOption(gopdf.PageOption{PageSize: pageSize})
	if err := p.pdf.ImageByHolder(imageHolder, 0, 0, nil); err != nil {
		return fmt.Errorf("failed to add image to PDF: %w", err)
	}
	return nil
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
	}
}
