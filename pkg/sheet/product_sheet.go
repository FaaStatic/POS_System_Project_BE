package sheet

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

type ProductRow struct {
	Line             int
	SKU              string
	Barcode          string
	ProductName      string
	Unit             string
	PurchasePrice    float64
	SellingPrice     float64
	SellingPriceDebt float64
	Stock            float64
	Category         string
	Image            string
	ProductType      string
}

type RowError struct {
	Line    int
	Message string
}

var requiredHeaders = []string{"sku", "product_name", "purchase_price", "selling_price", "selling_price_debt"}

const maxImportRows = 10000

var errTooManyRows = fmt.Errorf("file has more than %d rows", maxImportRows)

var xlsxLimits = excelize.Options{UnzipSizeLimit: 200 << 20, UnzipXMLSizeLimit: 16 << 20}

func ParseProducts(r io.Reader, filename string) ([]ProductRow, []RowError, error) {
	ext := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(ext, ".csv"):
		return parseRecords(readCSV(r))
	case strings.HasSuffix(ext, ".xlsx"):
		records, err := readXLSX(r)
		if err != nil {
			return nil, nil, err
		}
		return parseRecords(records, nil)
	default:
		return nil, nil, fmt.Errorf("unsupported file format (use .csv or .xlsx)")
	}
}

func readCSV(r io.Reader) ([][]string, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	var records [][]string
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			return records, nil
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read csv: %w", err)
		}
		if len(records) > maxImportRows {
			return nil, errTooManyRows
		}
		records = append(records, rec)
	}
}

func readXLSX(r io.Reader) ([][]string, error) {
	f, err := excelize.OpenReader(r, xlsxLimits)
	if err != nil {
		return nil, fmt.Errorf("failed to read excel: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("excel file has no sheet")
	}
	rows, err := f.Rows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to read excel rows: %w", err)
	}
	defer rows.Close()

	var records [][]string
	for rows.Next() {
		if len(records) > maxImportRows {
			return nil, errTooManyRows
		}
		cols, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("failed to read excel rows: %w", err)
		}
		records = append(records, cols)
	}
	if err := rows.Error(); err != nil {
		return nil, fmt.Errorf("failed to read excel rows: %w", err)
	}
	return records, nil
}

func parseRecords(records [][]string, err error) ([]ProductRow, []RowError, error) {
	if err != nil {
		return nil, nil, err
	}
	if len(records) == 0 {
		return nil, nil, fmt.Errorf("file is empty")
	}

	colIndex := make(map[string]int)
	for i, h := range records[0] {
		colIndex[normalizeHeader(h)] = i
	}
	for _, h := range requiredHeaders {
		if _, ok := colIndex[h]; !ok {
			return nil, nil, fmt.Errorf("required column '%s' not found in header", h)
		}
	}

	var rows []ProductRow
	var rowErrors []RowError

	for i := 1; i < len(records); i++ {
		line := i + 1
		rec := records[i]
		if isEmptyRecord(rec) {
			continue
		}

		get := func(key string) string {
			idx, ok := colIndex[key]
			if !ok || idx >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[idx])
		}

		row := ProductRow{
			Line:        line,
			SKU:         get("sku"),
			Barcode:     get("barcode"),
			ProductName: get("product_name"),
			Unit:        get("unit"),
			Category:    get("category"),
			Image:       get("image"),
			ProductType: get("product_type"),
		}

		if row.SKU == "" {
			rowErrors = append(rowErrors, RowError{Line: line, Message: "sku is empty"})
			continue
		}
		if row.ProductName == "" {
			rowErrors = append(rowErrors, RowError{Line: line, Message: "product_name is empty"})
			continue
		}

		var parseErr error
		if row.PurchasePrice, parseErr = parseNonNegativeFloat(get("purchase_price")); parseErr != nil {
			rowErrors = append(rowErrors, RowError{Line: line, Message: "invalid purchase_price"})
			continue
		}
		if row.SellingPrice, parseErr = parseNonNegativeFloat(get("selling_price")); parseErr != nil {
			rowErrors = append(rowErrors, RowError{Line: line, Message: "invalid selling_price"})
			continue
		}
		if row.SellingPriceDebt, parseErr = parseNonNegativeFloat(get("selling_price_debt")); parseErr != nil {
			rowErrors = append(rowErrors, RowError{Line: line, Message: "invalid selling_price_debt"})
			continue
		}
		if row.Stock, parseErr = parseNonNegativeFloat(get("stock")); parseErr != nil {
			rowErrors = append(rowErrors, RowError{Line: line, Message: "invalid stock"})
			continue
		}

		rows = append(rows, row)
	}

	return rows, rowErrors, nil
}

func normalizeHeader(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.ReplaceAll(h, " ", "_")
	h = strings.ReplaceAll(h, "-", "_")
	return h
}

func isEmptyRecord(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func parseNonNegativeFloat(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("value must not be negative")
	}
	return v, nil
}
