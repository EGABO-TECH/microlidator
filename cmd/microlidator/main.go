// Command microlidator is the single-binary CLI entry point. This file
// owns flag parsing, subcommand routing, ANSI decoration, and tabular
// printing — the "CLI / Terminal UI Layer" in the architecture diagram.
// All business logic lives in internal/service; all persistence lives in
// internal/storage.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"microlidator/internal/domain"
	"microlidator/internal/service"
	"microlidator/internal/storage"
)

const version = "1.0.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches to a subcommand and returns a process exit code. Keeping
// os.Exit out of the subcommand handlers means the same handlers can be
// reused from the "interactive" REPL without killing the session.
func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "init":
		return cmdInit(rest)
	case "add":
		return cmdAdd(rest)
	case "verify":
		return cmdVerify(rest)
	case "audit":
		return cmdAudit(rest)
	case "export-csv":
		return cmdExportCSV(rest)
	case "import-csv":
		return cmdImportCSV(rest)
	case "interactive":
		return cmdInteractive(rest)
	case "-h", "--help", "help":
		printUsage()
		return 0
	case "-v", "--version", "version":
		fmt.Println("microlidator", version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Println(`Microlidator - Enterprise Cryptographic Audit Ledger for Micro-Lenders & SACCOs

Usage:
  microlidator <command> [flags]

Commands:
  init          Initialize a new, empty SACCO ledger
  add           Append a new signed block to the ledger chain
  verify        Audit cryptographic hash integrity across all blocks
  audit         Generate a financial summary statement for a member
  export-csv    Export the ledger chain to a spreadsheet-compatible CSV
  import-csv    Rebuild and cryptographically re-sign the ledger from a CSV
  interactive   Launch an interactive terminal session
  version       Print version information

Run 'microlidator <command> -h' to see flags for a specific command.`)
}

// ---------------------------------------------------------------------
// ANSI Decorator Pattern: wraps stdout with color codes, auto-disabling
// when stdout is not a terminal (e.g. piped to a script) or when
// --no-color is passed.
// ---------------------------------------------------------------------

type colorizer struct{ enabled bool }

func newColorizer(noColor bool) *colorizer {
	enabled := !noColor
	if fi, err := os.Stdout.Stat(); err == nil {
		if (fi.Mode() & os.ModeCharDevice) == 0 {
			enabled = false // piped stdout: auto-disable
		}
	}
	return &colorizer{enabled: enabled}
}

func (c *colorizer) wrap(code, s string) string {
	if !c.enabled {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func (c *colorizer) Red(s string) string    { return c.wrap("31", s) }
func (c *colorizer) Green(s string) string  { return c.wrap("32", s) }
func (c *colorizer) Yellow(s string) string { return c.wrap("33", s) }
func (c *colorizer) Cyan(s string) string   { return c.wrap("36", s) }
func (c *colorizer) Bold(s string) string   { return c.wrap("1", s) }

func errNotFoundMsg() string {
	return "Ledger file not found. Run 'init' first."
}

// ---------------------------------------------------------------------
// init
// ---------------------------------------------------------------------

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	file := fs.String("file", "ledger.json", "path to the ledger file")
	org := fs.String("org", "", "organization / SACCO name (required)")
	currency := fs.String("currency", "UGX", "ledger currency code")
	noColor := fs.Bool("no-color", false, "disable ANSI colored output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c := newColorizer(*noColor)

	if *org == "" {
		fmt.Fprintln(os.Stderr, c.Red("Error: --org is required"))
		return 1
	}

	svc := service.NewLedgerService(storage.NewJSONRepo(*file))
	ledger, err := svc.Init(*org, strings.ToUpper(*currency))
	if err != nil {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+err.Error()))
		return 1
	}

	fmt.Println(c.Green("✓ Ledger initialized"))
	fmt.Printf("  Organization: %s\n", ledger.Organization)
	fmt.Printf("  Currency:     %s\n", ledger.Currency)
	fmt.Printf("  File:         %s\n", *file)
	return 0
}

// ---------------------------------------------------------------------
// add
// ---------------------------------------------------------------------

func cmdAdd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	file := fs.String("file", "ledger.json", "path to the ledger file")
	memberID := fs.String("member-id", "", "unique member ID, e.g. MB-001 (required)")
	member := fs.String("member", "", "full member name (required)")
	txType := fs.String("type", "", "DEPOSIT|WITHDRAWAL|LOAN_ISSUE|LOAN_REPAYMENT|INTEREST_PAID|FEE (required)")
	amount := fs.Float64("amount", 0, "transaction amount (required, > 0)")
	note := fs.String("note", "", "memo reference")
	noColor := fs.Bool("no-color", false, "disable ANSI colored output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c := newColorizer(*noColor)

	if *memberID == "" || *member == "" || *txType == "" {
		fmt.Fprintln(os.Stderr, c.Red("Error: --member-id, --member, and --type are required"))
		return 1
	}
	if !domain.ValidTransactionType(*txType) {
		fmt.Fprintln(os.Stderr, c.Red("Error: invalid --type "+*txType+
			" (expected DEPOSIT|WITHDRAWAL|LOAN_ISSUE|LOAN_REPAYMENT|INTEREST_PAID|FEE)"))
		return 1
	}

	repo := storage.NewJSONRepo(*file)
	if !repo.Exists() {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+errNotFoundMsg()))
		return 1
	}

	svc := service.NewLedgerService(repo)
	block, err := svc.AddBlock(*memberID, *member, domain.TransactionType(*txType), *amount, *note)
	if err != nil {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+err.Error()))
		return 1
	}

	fmt.Println(c.Green("✓ Block appended"))
	fmt.Printf("  Index: %d\n", block.Index)
	fmt.Printf("  Hash:  %s\n", block.Hash)
	return 0
}

// ---------------------------------------------------------------------
// verify
// ---------------------------------------------------------------------

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	file := fs.String("file", "ledger.json", "path to the ledger file")
	verbose := fs.Bool("verbose", false, "print remediation hints for each issue")
	jsonOut := fs.Bool("json", false, "output result as JSON")
	noColor := fs.Bool("no-color", false, "disable ANSI colored output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c := newColorizer(*noColor)

	repo := storage.NewJSONRepo(*file)
	if !repo.Exists() {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+errNotFoundMsg()))
		return 1
	}

	svc := service.NewLedgerService(repo)
	result, err := svc.Verify()
	if err != nil {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+err.Error()))
		return 1
	}

	if *jsonOut {
		printVerifyJSON(result)
	} else {
		printVerifyHuman(result, c, *verbose)
	}

	if !result.Valid {
		return 1
	}
	return 0
}

func printVerifyHuman(r *service.VerificationResult, c *colorizer, verbose bool) {
	fmt.Printf("Scanned %d block(s)\n\n", r.TotalBlocks)
	if r.Valid {
		fmt.Println(c.Green(c.Bold("✓ LEDGER INTEGRITY VERIFIED — no tampering detected")))
		return
	}

	fmt.Println(c.Red(c.Bold("✗ LEDGER INTEGRITY COMPROMISED")))
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "BLOCK\tRULE\tDETAIL")
	for _, issue := range r.Issues {
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Red(strconv.Itoa(issue.BlockIndex)), issue.Rule, issue.Detail)
	}
	w.Flush()
	if verbose {
		fmt.Println()
		fmt.Println(c.Yellow("Run 'audit --member-id <id>' to inspect the histories of affected members."))
	}
}

func printVerifyJSON(r *service.VerificationResult) {
	var b strings.Builder
	fmt.Fprintf(&b, "{\n  \"total_blocks\": %d,\n  \"valid\": %t,\n  \"issues\": [\n", r.TotalBlocks, r.Valid)
	for i, issue := range r.Issues {
		comma := ","
		if i == len(r.Issues)-1 {
			comma = ""
		}
		fmt.Fprintf(&b, "    {\"block_index\": %d, \"rule\": %q, \"detail\": %q}%s\n",
			issue.BlockIndex, issue.Rule, issue.Detail, comma)
	}
	b.WriteString("  ]\n}")
	fmt.Println(b.String())
}

// ---------------------------------------------------------------------
// audit
// ---------------------------------------------------------------------

func cmdAudit(args []string) int {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	file := fs.String("file", "ledger.json", "path to the ledger file")
	memberID := fs.String("member-id", "", "member ID to summarize (required)")
	noColor := fs.Bool("no-color", false, "disable ANSI colored output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c := newColorizer(*noColor)

	if *memberID == "" {
		fmt.Fprintln(os.Stderr, c.Red("Error: --member-id is required"))
		return 1
	}

	repo := storage.NewJSONRepo(*file)
	if !repo.Exists() {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+errNotFoundMsg()))
		return 1
	}

	svc := service.NewLedgerService(repo)
	summary, err := svc.Audit(*memberID)
	if err != nil {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+err.Error()))
		return 1
	}

	fmt.Println(c.Bold(c.Cyan(fmt.Sprintf("Member Summary: %s (%s)", summary.Member, summary.MemberID))))
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "Transactions:\t%d\n", summary.TransactionCount)
	fmt.Fprintf(w, "Total Deposits:\t%.2f\n", summary.TotalDeposits)
	fmt.Fprintf(w, "Total Withdrawals:\t%.2f\n", summary.TotalWithdrawals)
	fmt.Fprintf(w, "Total Loans Issued:\t%.2f\n", summary.TotalLoansIssued)
	fmt.Fprintf(w, "Total Repayments:\t%.2f\n", summary.TotalRepayments)
	fmt.Fprintf(w, "Total Interest Paid:\t%.2f\n", summary.TotalInterest)
	fmt.Fprintf(w, "Total Fees:\t%.2f\n", summary.TotalFees)
	fmt.Fprintf(w, "Net Balance:\t%.2f\n", summary.NetBalance)
	w.Flush()
	return 0
}

// ---------------------------------------------------------------------
// export-csv
// ---------------------------------------------------------------------

func cmdExportCSV(args []string) int {
	fs := flag.NewFlagSet("export-csv", flag.ContinueOnError)
	file := fs.String("file", "ledger.json", "path to the ledger file")
	out := fs.String("out", "ledger_export.csv", "output CSV path")
	noColor := fs.Bool("no-color", false, "disable ANSI colored output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c := newColorizer(*noColor)

	repo := storage.NewJSONRepo(*file)
	if !repo.Exists() {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+errNotFoundMsg()))
		return 1
	}

	ledger, err := repo.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+err.Error()))
		return 1
	}
	if err := storage.ExportCSV(ledger, *out); err != nil {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+err.Error()))
		return 1
	}

	fmt.Println(c.Green(fmt.Sprintf("✓ Exported %d block(s) to %s", len(ledger.Blocks), *out)))
	return 0
}

// ---------------------------------------------------------------------
// import-csv
// ---------------------------------------------------------------------

func cmdImportCSV(args []string) int {
	fs := flag.NewFlagSet("import-csv", flag.ContinueOnError)
	file := fs.String("file", "ledger.json", "path to the ledger file to (re)build")
	in := fs.String("in", "", "input CSV path (required)")
	org := fs.String("org", "", "organization / SACCO name (required if no ledger file exists yet)")
	currency := fs.String("currency", "UGX", "ledger currency code (used only when creating a fresh ledger)")
	noColor := fs.Bool("no-color", false, "disable ANSI colored output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c := newColorizer(*noColor)

	if *in == "" {
		fmt.Fprintln(os.Stderr, c.Red("Error: --in is required"))
		return 1
	}

	repo := storage.NewJSONRepo(*file)
	orgName, curr := *org, strings.ToUpper(*currency)
	if repo.Exists() {
		if existing, err := repo.Load(); err == nil {
			if orgName == "" {
				orgName = existing.Organization
			}
			curr = existing.Currency
		}
	}
	if orgName == "" {
		fmt.Fprintln(os.Stderr, c.Red("Error: --org is required when no existing ledger is present"))
		return 1
	}

	blocks, err := storage.ImportCSV(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+err.Error()))
		return 1
	}

	svc := service.NewLedgerService(repo)
	if err := svc.Rebuild(orgName, curr, blocks); err != nil {
		fmt.Fprintln(os.Stderr, c.Red("Error: "+err.Error()))
		return 1
	}

	fmt.Println(c.Green(fmt.Sprintf("✓ Rebuilt and re-signed ledger with %d block(s)", len(blocks))))
	return 0
}

// ---------------------------------------------------------------------
// interactive
// ---------------------------------------------------------------------

func cmdInteractive(args []string) int {
	fs := flag.NewFlagSet("interactive", flag.ContinueOnError)
	file := fs.String("file", "ledger.json", "path to the ledger file")
	noColor := fs.Bool("no-color", false, "disable ANSI colored output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	c := newColorizer(*noColor)

	reader := bufio.NewReader(os.Stdin)
	fmt.Println(c.Bold(c.Cyan("Microlidator Interactive Session")) + " — type 'help' for commands, 'exit' to quit")

	for {
		fmt.Print(c.Cyan("microlidator> "))
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		sub, subArgs := fields[0], fields[1:]

		switch sub {
		case "exit", "quit":
			fmt.Println("Goodbye.")
			return 0
		case "help":
			fmt.Println("Commands: init, add, verify, audit, export-csv, import-csv, exit")
		case "init", "add", "verify", "audit", "export-csv", "import-csv":
			if !hasFileFlag(subArgs) {
				subArgs = append(subArgs, "--file", *file)
			}
			run(append([]string{sub}, subArgs...))
		default:
			fmt.Println(c.Yellow("Unknown command: " + sub))
		}
	}
	return 0
}

func hasFileFlag(args []string) bool {
	for _, a := range args {
		if a == "--file" || strings.HasPrefix(a, "--file=") {
			return true
		}
	}
	return false
}
