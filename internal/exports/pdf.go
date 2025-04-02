package exports

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"sync"

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
	pdf.SetCompressLevel(0)
	return &PDFGenerator{pdf: pdf}
}

func (p *PDFGenerator) AddImageToPDF(imgBytes []byte) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(imgBytes) == 0 {
		return errors.New("empty image data")
	}

	imageHolder, err := gopdf.ImageHolderByBytes(imgBytes)
	if err != nil {
		return fmt.Errorf("failed to create image holder: %w", err)
	}

	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(imgBytes))
	if err != nil {
		return fmt.Errorf("failed to decode image config: %w", err)
	}

	pageSize := &gopdf.Rect{
		W: float64(imageConfig.Width)*72/128 - 1,
		H: float64(imageConfig.Height)*72/128 - 1,
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

func (p *PDFGenerator) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.pdf != nil {
		p.pdf = nil
	}
}
