
<div align="center">
  <img src="assets/microlidator.jpg" alt="Microlidator logo" width="240">
</div>

# Microlidator

Enterprise cryptographic audit ledger for micro-lenders and SACCOs.
Single-binary Go CLI, zero external dependencies (standard library only).

## Build

```
go build -o microlidator ./cmd/microlidator
```

## Run tests

```
go test ./...
```

## Usage

```
microlidator init --org "Kampala Unity SACCO" --currency UGX --file ledger.json

microlidator add --file ledger.json \
  --member-id MB-001 --member "Grace Nakato" \
  --type DEPOSIT --amount 150000 --note "Weekly savings"

microlidator verify --file ledger.json
microlidator audit --file ledger.json --member-id MB-001
microlidator export-csv --file ledger.json --out ledger.csv
microlidator import-csv --file ledger.json --in ledger.csv
microlidator interactive --file ledger.json
```

Transaction types: `DEPOSIT`, `WITHDRAWAL`, `LOAN_ISSUE`, `LOAN_REPAYMENT`,
`INTEREST_PAID`, `FEE`.

## Layout

```
cmd/microlidator/main.go        CLI layer (flags, subcommands, ANSI/tabwriter output)
internal/domain/block.go        Block/Ledger types, canonical SHA-256 hashing
internal/storage/json_repo.go   Atomic JSON persistence (temp file + os.Rename)
internal/storage/csv_adapter.go CSV export/import
internal/service/ledger_service.go  Init/Add/Verify/Audit business logic
```

## Notes on the spec

- `verify` collects *every* rule violation across the chain in one pass
  (genesis check, sequence check, hash-linkage check, content-integrity
  check) rather than stopping at the first mismatch, so a single run gives
  a complete tampering report.
- `import-csv` treats the CSV's member/transaction/amount data as source of
  truth but always re-derives `index`, `prev_hash`, and `hash` sequentially,
  so a hand-edited CSV can never be replayed into an inconsistent chain.
- All CLI subcommand handlers return an exit code instead of calling
  `os.Exit` directly, so the same handlers are reused by `interactive`
  without killing the REPL session on a validation error.
