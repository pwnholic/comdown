package exports

type documentExporter struct {
	PDF interface {
		AddImageToPDF(imgBytes []byte) error
		SavePDF(outputPath string) error
	}
}

func NewDocumentExporter() *documentExporter {
	return &documentExporter{
		PDF: NewPDFGenerator(),
	}
}
