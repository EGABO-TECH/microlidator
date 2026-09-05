// Package domain contains the core ledger data model and the cryptographic
// hashing rules that keep the block chain tamper-evident. It has no
// dependency on storage or CLI concerns (Clean Architecture: domain core).
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// TransactionType enumerates the financial operations Microlidator can record.
type TransactionType string

const (
	TypeDeposit       TransactionType = "DEPOSIT"
	TypeWithdrawal    TransactionType = "WITHDRAWAL"
	TypeLoanIssue     TransactionType = "LOAN_ISSUE"
	TypeLoanRepayment TransactionType = "LOAN_REPAYMENT"
	TypeInterestPaid  TransactionType = "INTEREST_PAID"
	TypeFee           TransactionType = "FEE"
)

// ValidTransactionType reports whether s is one of the known transaction types.
func ValidTransactionType(s string) bool {
	switch TransactionType(s) {
	case TypeDeposit, TypeWithdrawal, TypeLoanIssue, TypeLoanRepayment, TypeInterestPaid, TypeFee:
		return true
	default:
		return false
	}
}

// Block represents a single cryptographically linked ledger entry.
type Block struct {
	Index     int             `json:"index"`      // Sequential identifier (1-based)
	Timestamp string          `json:"timestamp"`  // UTC RFC3339 string
	MemberID  string          `json:"member_id"`  // Unique ID (e.g., "MB-001")
	Member    string          `json:"member"`     // Full human-readable name
	Type      TransactionType `json:"type"`       // Transaction category
	Amount    float64         `json:"amount"`     // Financial value
	Note      string          `json:"note"`       // Memo reference
	PrevHash  string          `json:"prev_hash"`  // SHA-256 hex digest of block N-1
	Hash      string          `json:"hash"`       // SHA-256 hex digest of current block
}

// Ledger wraps metadata and the immutable transaction chain.
type Ledger struct {
	Organization string  `json:"organization"` // SACCO / Group Name
	Currency     string  `json:"currency"`     // E.g., "UGX", "KES", "USD"
	CreatedAt    string  `json:"created_at"`   // Creation RFC3339 timestamp
	Version      string  `json:"version"`      // Schema version ("1.0")
	Blocks       []Block `json:"blocks"`       // Ordered block list
}

// CalculateHash computes the deterministic SHA-256 digest of a block using
// the canonical pipe-delimited payload defined in the spec:
//
//	Index|Timestamp|MemberID|Member|Type|Amount|Note|PrevHash
//
// Amount is always formatted to two decimal places so the digest is stable
// across systems regardless of floating point representation quirks.
func CalculateHash(b Block) string {
	record := fmt.Sprintf("%d|%s|%s|%s|%s|%.2f|%s|%s",
		b.Index, b.Timestamp, b.MemberID, b.Member, b.Type, b.Amount, b.Note, b.PrevHash,
	)
	h := sha256.New()
	h.Write([]byte(record))
	return hex.EncodeToString(h.Sum(nil))
}
