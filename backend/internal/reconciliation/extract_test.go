package reconciliation

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExcelPreservesSourceValuesAndSheets(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	book.SetSheetRow("Sheet1", "A1", &[]string{"Date", "Reference", "Debit", "Credit"})
	book.SetSheetRow("Sheet1", "A2", &[]string{"06/11/2026", "0000123", "1,234.50", ""})
	book.NewSheet("Supplier")
	book.SetSheetRow("Supplier", "A3", &[]string{"Ticket", "1231234567890", "-20.00"})
	data, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := Extract(context.Background(), "statement.xlsx", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[1].Cells[1] != "0000123" || rows[1].Cells[2] != "1,234.50" || rows[2].Section != "Supplier" || rows[2].SourceRow != 3 {
		t.Fatalf("source values changed: %#v", rows)
	}
}
func TestCSVRetainsQuotedAmountsAndBlankColumns(t *testing.T) {
	rows, err := Extract(context.Background(), "statement.csv", []byte("\xef\xbb\xbfDate,Reference,Debit,Credit\n06/11/2026,0000123,\"1,234.50\",\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1].Cells[2] != "1,234.50" || rows[1].Cells[3] != "" {
		t.Fatalf("incorrect cells: %#v", rows)
	}
}
func TestPDFExtractsVisibleText(t *testing.T) {
	data := testPDF("BT /F1 12 Tf 40 750 Td (Date Reference Debit Credit) Tj 0 -20 Td (06/11/2026 REF001 1234.50 0.00) Tj ET")
	rows, err := Extract(context.Background(), "statement.pdf", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1].Section != "Page 1" || !strings.Contains(strings.Join(rows[1].Cells, " "), "REF001") {
		t.Fatalf("PDF text missing: %#v", rows)
	}
}
func TestRejectsUnreadableStatementsAndLimits(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty.csv", nil}, {"old.xls", []byte("not an xlsx workbook")}, {"bad.xlsx", []byte("not a zip")}, {"bad.pdf", []byte("not a PDF")},
		{"scan.pdf", testPDF("")}, {"too-many.csv", []byte(strings.Repeat("value\n", maxRows+1))}, {"big.csv", bytes.Repeat([]byte("x"), MaxFileSize+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Extract(context.Background(), tt.name, tt.data); err == nil {
				t.Fatal("unreadable/over-limit input accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Extract(ctx, "statement.csv", []byte("value")); err == nil {
		t.Fatal("cancelled extraction accepted")
	}
}
func testPDF(content string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	start := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), start)
	return buf.Bytes()
}
