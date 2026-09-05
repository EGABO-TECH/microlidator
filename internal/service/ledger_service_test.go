package service

import (
	"os"
	"path/filepath"
	"testing"

	"microlidator/internal/domain"
	"microlidator/internal/storage"
)

func newTestService(t *testing.T) (*LedgerService, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.json")
	svc := NewLedgerService(storage.NewJSONRepo(path))
	return svc, path
}

func TestInitAndAddBuildsValidChain(t *testing.T) {
	svc, _ := newTestService(t)

	if _, err := svc.Init("Test SACCO", "UGX"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := svc.AddBlock("MB-001", "Alice", domain.TypeDeposit, 1000, "first"); err != nil {
		t.Fatalf("AddBlock 1: %v", err)
	}
	if _, err := svc.AddBlock("MB-001", "Alice", domain.TypeWithdrawal, 200, "second"); err != nil {
		t.Fatalf("AddBlock 2: %v", err)
	}

	result, err := svc.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid chain, got issues: %+v", result.Issues)
	}
	if result.TotalBlocks != 2 {
		t.Fatalf("expected 2 blocks, got %d", result.TotalBlocks)
	}
}

func TestAddRejectsNonPositiveAmount(t *testing.T) {
	svc, _ := newTestService(t)
	svc.Init("Test SACCO", "UGX")

	if _, err := svc.AddBlock("MB-001", "Alice", domain.TypeDeposit, 0, "zero"); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if _, err := svc.AddBlock("MB-001", "Alice", domain.TypeDeposit, -10, "negative"); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestVerifyDetectsTamperedPayload(t *testing.T) {
	svc, path := newTestService(t)
	svc.Init("Test SACCO", "UGX")
	svc.AddBlock("MB-001", "Alice", domain.TypeDeposit, 1000, "first")

	repo := storage.NewJSONRepo(path)
	ledger, _ := repo.Load()
	ledger.Blocks[0].Amount = 999999 // tamper without recomputing hash
	if err := repo.Save(ledger); err != nil {
		t.Fatalf("Save: %v", err)
	}

	result, err := svc.Verify()
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Valid {
		t.Fatal("expected tampering to be detected")
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Rule == "TamperedBlock" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a TamperedBlock issue, got: %+v", result.Issues)
	}
}

func TestVerifyDetectsBrokenLink(t *testing.T) {
	svc, path := newTestService(t)
	svc.Init("Test SACCO", "UGX")
	svc.AddBlock("MB-001", "Alice", domain.TypeDeposit, 1000, "first")
	svc.AddBlock("MB-001", "Alice", domain.TypeDeposit, 500, "second")

	repo := storage.NewJSONRepo(path)
	ledger, _ := repo.Load()
	ledger.Blocks[1].PrevHash = "deadbeef" // sever the chain link
	repo.Save(ledger)

	result, _ := svc.Verify()
	if result.Valid {
		t.Fatal("expected broken link to be detected")
	}
}

func TestAuditSummarizesPerMember(t *testing.T) {
	svc, _ := newTestService(t)
	svc.Init("Test SACCO", "UGX")
	svc.AddBlock("MB-001", "Alice", domain.TypeDeposit, 1000, "")
	svc.AddBlock("MB-001", "Alice", domain.TypeLoanIssue, 5000, "")
	svc.AddBlock("MB-001", "Alice", domain.TypeLoanRepayment, 1000, "")
	svc.AddBlock("MB-002", "Bob", domain.TypeDeposit, 300, "")

	summary, err := svc.Audit("MB-001")
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if summary.TransactionCount != 3 {
		t.Fatalf("expected 3 transactions, got %d", summary.TransactionCount)
	}
	wantNet := 1000.0 + 5000.0 - 1000.0
	if summary.NetBalance != wantNet {
		t.Fatalf("expected net balance %.2f, got %.2f", wantNet, summary.NetBalance)
	}

	if _, err := svc.Audit("MB-999"); err == nil {
		t.Fatal("expected error for unknown member")
	}
}

func TestCalculateHashIsDeterministic(t *testing.T) {
	b := domain.Block{
		Index: 1, Timestamp: "2026-01-01T00:00:00Z", MemberID: "MB-001",
		Member: "Alice", Type: domain.TypeDeposit, Amount: 1000.5, Note: "n", PrevHash: "0",
	}
	h1 := domain.CalculateHash(b)
	h2 := domain.CalculateHash(b)
	if h1 != h2 {
		t.Fatal("expected identical hashes for identical block content")
	}
	b.Amount = 1000.51
	if domain.CalculateHash(b) == h1 {
		t.Fatal("expected different hash after amount change")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
