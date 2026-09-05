package storage

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"microlidator/internal/domain"
)

var csvHeader = []string{
	"index", "timestamp", "member_id", "member", "type", "amount", "note", "prev_hash", "hash",
}

// ExportCSV writes the full ledger chain to a spreadsheet-compatible CSV file.
func ExportCSV(ledger *domain.Ledger, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating csv file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write(csvHeader); err != nil {
		return fmt.Errorf("writing csv header: %w", err)
	}
	for _, b := range ledger.Blocks {
		row := []string{
			strconv.Itoa(b.Index),
			b.Timestamp,
			b.MemberID,
			b.Member,
			string(b.Type),
			strconv.FormatFloat(b.Amount, 'f', 2, 64),
			b.Note,
			b.PrevHash,
			b.Hash,
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("writing csv row: %w", err)
		}
	}
	return w.Error()
}

// ImportCSV rebuilds a block chain from a CSV file. It trusts the CSV's
// member/transaction/amount data but discards any stored index/prev_hash/hash
// columns and re-derives them sequentially, so the resulting chain is always
// cryptographically self-consistent regardless of how the CSV was edited.
func ImportCSV(inPath string) ([]domain.Block, error) {
	f, err := os.Open(inPath)
	if err != nil {
		return nil, fmt.Errorf("opening csv file: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading csv: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("csv file is empty")
	}

	dataRows := records[1:] // skip header
	blocks := make([]domain.Block, 0, len(dataRows))
	prevHash := "0"

	for i, row := range dataRows {
		if len(row) < 7 {
			return nil, fmt.Errorf("row %d: expected at least 7 columns (index,timestamp,member_id,member,type,amount,note)", i+2)
		}
		amount, err := strconv.ParseFloat(row[5], 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid amount %q: %w", i+2, row[5], err)
		}
		txType := domain.TransactionType(row[4])
		if !domain.ValidTransactionType(string(txType)) {
			return nil, fmt.Errorf("row %d: invalid transaction type %q", i+2, row[4])
		}

		b := domain.Block{
			Index:     i + 1,
			Timestamp: row[1],
			MemberID:  row[2],
			Member:    row[3],
			Type:      txType,
			Amount:    amount,
			Note:      row[6],
			PrevHash:  prevHash,
		}
		b.Hash = domain.CalculateHash(b)
		blocks = append(blocks, b)
		prevHash = b.Hash
	}
	return blocks, nil
}
