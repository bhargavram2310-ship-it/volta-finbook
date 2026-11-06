package reconciliation

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
)

const MaxFileSize = 10 << 20
const maxRows = 10000
const maxExtractedBytes = 16 << 20

type Row struct {
	Number    int      `json:"number"`
	Section   string   `json:"section"`
	SourceRow int      `json:"sourceRow"`
	Cells     []string `json:"cells"`
}

// Extract preserves source values, including header and balance rows. It does
// not infer dates, round money, execute formulas, or create ledger postings.
func Extract(ctx context.Context, name string, data []byte) (result []Row, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = errors.New("The statement could not be read. Export a new PDF or Excel workbook and try again.")
		}
	}()
	if len(data) == 0 || len(data) > MaxFileSize {
		return nil, errors.New("Choose a nonempty statement up to 10 MB.")
	}
	result = []Row{}
	size := 0
	appendRow := func(section string, number int, cells []string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		nonempty := false
		if len(cells) > 256 {
			return errors.New("Statement exceeds 256 columns.")
		}
		for _, value := range cells {
			if !utf8.ValidString(value) {
				return errors.New("Statement contains invalid text encoding. Export a UTF-8 CSV or a new workbook.")
			}
			if strings.TrimSpace(value) != "" {
				nonempty = true
			}
			size += len(value)
		}
		if size > maxExtractedBytes {
			return errors.New("Extracted statement exceeds 16 MB.")
		}
		if !nonempty {
			return nil
		}
		if len(result) >= maxRows {
			return errors.New("Statement exceeds 10,000 nonempty rows. Split it into smaller files.")
		}
		result = append(result, Row{len(result) + 1, section, number, cells})
		return nil
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx":
		// Reject zip bombs before the workbook library opens any XML or temp files.
		z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if e != nil {
			return nil, errors.New("Invalid Excel workbook.")
		}
		var expanded uint64
		if len(z.File) > 2000 {
			return nil, errors.New("Workbook has too many archive entries.")
		}
		for _, f := range z.File {
			if f.UncompressedSize64 > 64<<20 || expanded > 64<<20-f.UncompressedSize64 {
				return nil, errors.New("Workbook exceeds the 64 MB expanded size limit.")
			}
			expanded += f.UncompressedSize64
		}
		book, e := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 16 << 20})
		if e != nil {
			return nil, errors.New("Excel workbook could not be read. Remove password protection and save as .xlsx.")
		}
		defer book.Close()
		for _, sheet := range book.GetSheetList() {
			rows, e := book.Rows(sheet)
			if e != nil {
				return nil, e
			}
			n := 0
			for rows.Next() {
				n++
				if n > 100000 {
					rows.Close()
					return nil, errors.New("Workbook has too many worksheet rows.")
				}
				cells, e := rows.Columns()
				if e == nil {
					e = appendRow(sheet, n, cells)
				}
				if e != nil {
					rows.Close()
					return nil, e
				}
			}
			e = rows.Error()
			rows.Close()
			if e != nil {
				return nil, e
			}
		}
	case ".pdf":
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return nil, errors.New("Invalid PDF statement.")
		}
		doc, e := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
		if e != nil {
			return nil, errors.New("PDF could not be read. Remove password protection and try again.")
		}
		if doc.NumPage() > 200 {
			return nil, errors.New("PDF exceeds 200 pages. Split it into smaller files.")
		}
		for page := 1; page <= doc.NumPage(); page++ {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			for n, cells := range pdfTextRows(doc.Page(page).Content().Text) {
				if e = appendRow(fmt.Sprintf("Page %d", page), n+1, cells); e != nil {
					return nil, e
				}
			}
		}
	case ".csv":
		reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
		reader.FieldsPerRecord = -1
		for n := 1; ; n++ {
			cells, e := reader.Read()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, errors.New("CSV could not be read. Check its quoting and delimiter.")
			}
			if e = appendRow("CSV", n, cells); e != nil {
				return nil, e
			}
		}
	default:
		return nil, errors.New("Supported files: PDF, Excel (.xlsx), and CSV. Save older .xls files as .xlsx first.")
	}
	if len(result) == 0 {
		return nil, errors.New("No readable statement rows found. Scanned PDFs need OCR; upload a searchable PDF or an Excel export.")
	}
	return result, nil
}

// Use positioned glyphs so text matrices and multiple text blocks retain their
// visual line order. PDF cell boundaries remain an extraction aid for review.
func pdfTextRows(text []pdf.Text) [][]string {
	lines := map[int64][]pdf.Text{}
	positions := []int64{}
	for _, glyph := range text {
		position := int64(math.Round(glyph.Y))
		if _, ok := lines[position]; !ok {
			positions = append(positions, position)
		}
		lines[position] = append(lines[position], glyph)
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i] > positions[j] })
	rows := [][]string{}
	for _, position := range positions {
		line := lines[position]
		sort.SliceStable(line, func(i, j int) bool { return line[i].X < line[j].X })
		cells := []string{}
		var cell strings.Builder
		var previous pdf.Text
		for i, glyph := range line {
			if i > 0 {
				gap := glyph.X - previous.X - previous.W
				if gap > math.Max(8, glyph.FontSize) {
					cells = append(cells, cell.String())
					cell.Reset()
				} else if gap > glyph.FontSize*0.2 {
					cell.WriteByte(' ')
				}
			}
			cell.WriteString(glyph.S)
			previous = glyph
		}
		if cell.Len() > 0 {
			cells = append(cells, cell.String())
		}
		rows = append(rows, cells)
	}
	return rows
}
