package exports

import (
	"bytes"
	"image"

	"github.com/pwnholic/comdown/internal"
	"github.com/signintech/gopdf"
)

type pdfGenerator struct {
	PDF *gopdf.GoPdf
}

func NewPDFGenerator() *pdfGenerator {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{
		Unit:     gopdf.UnitPT,
		PageSize: *gopdf.PageSizeA4,
	})
	return &pdfGenerator{PDF: &pdf}
}

func (p *pdfGenerator) AddImageToPDF(imgBytes []byte) error {
	imageHolder, err := gopdf.ImageHolderByBytes(imgBytes)
	if err != nil {
		internal.ErrorLog("Failed to create image holder: %s", err.Error())
		return err
	}

	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(imgBytes))
	if err != nil {
		internal.ErrorLog("Failed to decode image config: %s", err.Error())
		return err
	}

	p.PDF.AddPageWithOption(gopdf.PageOption{PageSize: &gopdf.Rect{
		W: float64(imageConfig.Width)*72/128 - 1,
		H: float64(imageConfig.Height)*72/128 - 1,
	}})
	return p.PDF.ImageByHolder(imageHolder, 0, 0, nil)
}

func (p *pdfGenerator) SavePDF(outputPath string) error {
	internal.InfoLog("Saving PDF to: %s", outputPath)
	return p.PDF.WritePdf(outputPath)
}
