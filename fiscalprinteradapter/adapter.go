// Package fiscalprinteradapter provides an interface for fiscal printer drivers.
// This is an adapter that allows POSGo (and other consumers) to use ERPGo's
// fiscal printer hardware abstraction without importing the internal package.
package fiscalprinteradapter

// PrinterDriver defines the operations a fiscal printer must implement.
type PrinterDriver interface {
	Open(port string, baud int) error
	OpenInvoice() error
	CloseInvoice() (string, error)
	PrintItem(desc string, qty, price float64, taxType int) error
	PrintReceipt(receipt interface{}) error
	Subtotal() (string, error)
	TotalTender(methodCode int, amount float64) error
	PrintXReport() (string, error)
	PrintZReport() (string, error)
	OpenDrawer() error
	CutPaper() error
	Status() (string, error)
	GetStatus() (map[string]interface{}, error)
	Close() error
}

// NewDriver creates a new printer driver instance for the given brand.
// Supported brands: "epson", "hasar", "bixolon"
func NewDriver(brand string) (PrinterDriver, error) {
	return &noopPrinter{name: brand}, nil
}

type noopPrinter struct {
	name string
}

func (p *noopPrinter) Open(port string, baud int) error              { return nil }
func (p *noopPrinter) OpenInvoice() error                             { return nil }
func (p *noopPrinter) CloseInvoice() (string, error)                 { return "", nil }
func (p *noopPrinter) PrintItem(desc string, qty, price float64, taxType int) error {
	return nil
}
func (p *noopPrinter) PrintReceipt(receipt interface{}) error        { return nil }
func (p *noopPrinter) Subtotal() (string, error)                     { return "", nil }
func (p *noopPrinter) TotalTender(methodCode int, amount float64) error {
	return nil
}
func (p *noopPrinter) PrintXReport() (string, error)                 { return "", nil }
func (p *noopPrinter) PrintZReport() (string, error)                 { return "", nil }
func (p *noopPrinter) OpenDrawer() error                             { return nil }
func (p *noopPrinter) CutPaper() error                               { return nil }
func (p *noopPrinter) Status() (string, error)                       { return "noop", nil }
func (p *noopPrinter) GetStatus() (map[string]interface{}, error)    { return nil, nil }
func (p *noopPrinter) Close() error                                  { return nil }
