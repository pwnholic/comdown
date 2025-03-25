package exports

type DocumentExporter struct {
	PDF interface {
		AddImageToPDF(imgBytes []byte) error
		SavePDF(outputPath string) error
	}
}

func NewDocumentExporter() *DocumentExporter {
	return &DocumentExporter{
		PDF: NewPDFGenerator(),
	}
}
