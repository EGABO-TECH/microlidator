// Package service implements the application layer: the ledger manager,
// the cryptographic audit engine, and member re-calculation loops. It
// orchestrates the domain core and storage layer but contains no CLI or
// I/O-format concerns of its own.
package service

import (
	"fmt"
	"time"

	"microlidator/internal/domain"
	"microlidator/internal/storage"
)

// LedgerService is the application service that mediates all ledger
// mutations and read-side queries.
type LedgerService struct {
	repo *storage.JSONRepo
}

// NewLedgerService wires a service instance to a concrete repository.
func NewLedgerService(repo *storage.JSONRepo) *LedgerService {
	return &LedgerService{repo: repo}
}

// Init creates a new, empty ledger file. It refuses to overwrite an
// existing ledger.
func (s *LedgerService) Init(org, currency string) (*domain.Ledger, error) {
	if s.repo.Exists() {
		return nil, fmt.Errorf("ledger file already exists: %s", s.repo.Path)
	}
	ledger := &domain.Ledger{
		Organization: org,
		Currency:     currency,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Version:      "1.0",
		Blocks:       []domain.Block{},
	}
	if err := s.repo.Save(ledger); err != nil {
		return nil, err
	}
	return ledger, nil
}

// AddBlock appends a new cryptographically signed block to the chain.
func (s *LedgerService) AddBlock(memberID, member string, txType domain.TransactionType, amount float64, note string) (*domain.Block, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be greater than zero")
	}
	if !domain.ValidTransactionType(string(txType)) {
		return nil, fmt.Errorf("invalid transaction type: %s", txType)
	}

	ledger, err := s.repo.Load()
	if err != nil {
		return nil, err
	}

	prevHash := "0"
	index := 1
	if n := len(ledger.Blocks); n > 0 {
		last := ledger.Blocks[n-1]
		prevHash = last.Hash
		index = last.Index + 1
	}

	b := domain.Block{
		Index:     index,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		MemberID:  memberID,
		Member:    member,
		Type:      txType,
		Amount:    amount,
		Note:      note,
		PrevHash:  prevHash,
	}
	b.Hash = domain.CalculateHash(b)

	ledger.Blocks = append(ledger.Blocks, b)
	if err := s.repo.Save(ledger); err != nil {
		return nil, err
	}
	return &b, nil
}

// VerificationIssue describes a single rule violation found during Verify.
type VerificationIssue struct {
	BlockIndex int
	Rule       string
	Detail     string
}

// VerificationResult is the outcome of running the four-rule audit engine.
type VerificationResult struct {
	TotalBlocks int
	Valid       bool
	Issues      []VerificationIssue
}

// Verify runs the four sequential integrity checks described in the spec:
//  1. Genesis Block Rule
//  2. Chain Sequence Rule
//  3. Hash Linkage Check
//  4. Data Content Integrity Check
//
// It does not stop at the first mismatch — it collects every issue found
// across the whole chain so a single verify run gives a complete tampering
// report.
func (s *LedgerService) Verify() (*VerificationResult, error) {
	ledger, err := s.repo.Load()
	if err != nil {
		return nil, err
	}

	result := &VerificationResult{TotalBlocks: len(ledger.Blocks), Valid: true}
	fail := func(idx int, rule, detail string) {
		result.Valid = false
		result.Issues = append(result.Issues, VerificationIssue{idx, rule, detail})
	}

	for i, b := range ledger.Blocks {
		if i == 0 {
			if b.Index != 1 {
				fail(b.Index, "GenesisIndex", "first block index must be 1")
			}
			if b.PrevHash != "0" {
				fail(b.Index, "GenesisPrevHash", `genesis prev_hash must be "0"`)
			}
		} else {
			prev := ledger.Blocks[i-1]
			if b.Index != prev.Index+1 {
				fail(b.Index, "SequenceBreak", fmt.Sprintf("expected index %d, got %d", prev.Index+1, b.Index))
			}
			if b.PrevHash != prev.Hash {
				fail(b.Index, "BrokenLink", "prev_hash does not match the previous block's hash")
			}
		}

		if computed := domain.CalculateHash(b); computed != b.Hash {
			fail(b.Index, "TamperedBlock", fmt.Sprintf("stored hash %s does not match recomputed hash %s", b.Hash, computed))
		}
	}

	return result, nil
}

// MemberSummary is a per-member financial statement produced by Audit.
type MemberSummary struct {
	MemberID         string
	Member           string
	TotalDeposits    float64
	TotalWithdrawals float64
	TotalLoansIssued float64
	TotalRepayments  float64
	TotalInterest    float64
	TotalFees        float64
	NetBalance       float64
	TransactionCount int
}

// Audit walks the chain and produces a financial summary for one member.
func (s *LedgerService) Audit(memberID string) (*MemberSummary, error) {
	ledger, err := s.repo.Load()
	if err != nil {
		return nil, err
	}

	summary := &MemberSummary{MemberID: memberID}
	found := false

	for _, b := range ledger.Blocks {
		if b.MemberID != memberID {
			continue
		}
		found = true
		summary.Member = b.Member
		summary.TransactionCount++

		switch b.Type {
		case domain.TypeDeposit:
			summary.TotalDeposits += b.Amount
			summary.NetBalance += b.Amount
		case domain.TypeWithdrawal:
			summary.TotalWithdrawals += b.Amount
			summary.NetBalance -= b.Amount
		case domain.TypeLoanIssue:
			summary.TotalLoansIssued += b.Amount
			summary.NetBalance += b.Amount
		case domain.TypeLoanRepayment:
			summary.TotalRepayments += b.Amount
			summary.NetBalance -= b.Amount
		case domain.TypeInterestPaid:
			summary.TotalInterest += b.Amount
			summary.NetBalance -= b.Amount
		case domain.TypeFee:
			summary.TotalFees += b.Amount
			summary.NetBalance -= b.Amount
		}
	}

	if !found {
		return nil, fmt.Errorf("no transactions found for member_id %q", memberID)
	}
	return summary, nil
}

// Rebuild replaces the ledger's block chain (used by import-csv) and
// persists the result atomically.
func (s *LedgerService) Rebuild(org, currency string, blocks []domain.Block) error {
	ledger := &domain.Ledger{
		Organization: org,
		Currency:     currency,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Version:      "1.0",
		Blocks:       blocks,
	}
	return s.repo.Save(ledger)
}
