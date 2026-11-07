package ledger

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

const (
	maxMemoLen = 300
	maxLines   = 200
)

var (
	sourceTypePattern = regexp.MustCompile(`^[a-z][a-z_.]{0,39}$`)
	codePattern       = regexp.MustCompile(`^[0-9]{4}$`)
)

// Line is one side of one account. Exactly one of Debit and Credit is positive.
type Line struct {
	AccountCode string
	Debit       money.Money
	Credit      money.Money
}

// Entry is a balanced set of lines describing one business event.
type Entry struct {
	OccurredOn time.Time
	Memo       string
	SourceType string // what caused it, such as "challan.released" or "payment.recorded"
	SourceID   string // the id of that thing (optional)
	Lines      []Line
}

// validateEntry checks everything that can be checked without the database: the header, each
// line, and that total debits equal total credits. The database repeats the balance check at
// commit (migration 00005), so this is the friendly check and that is the guarantee.
func validateEntry(e Entry) error {
	var fields []apperr.Field
	add := func(field, msg string) { fields = append(fields, apperr.Field{Field: field, Message: msg}) }

	if m := strings.TrimSpace(e.Memo); m == "" || len(m) > maxMemoLen {
		add("memo", "Required, up to 300 characters.")
	}
	if e.OccurredOn.IsZero() {
		add("occurred_on", "Required.")
	}
	if !sourceTypePattern.MatchString(e.SourceType) {
		add("source_type", "Lower-case letters, dots and underscores, such as payment.recorded.")
	}
	if e.SourceID != "" {
		if _, err := db.ParseUUID(e.SourceID); err != nil {
			add("source_id", "Must be an id.")
		}
	}
	if n := len(e.Lines); n < 2 || n > maxLines {
		add("lines", "An entry needs between 2 and 200 lines.")
	}

	var debit, credit money.Money
	overflow := false
	for i, l := range e.Lines {
		field := fmt.Sprintf("lines[%d]", i)
		switch {
		case !codePattern.MatchString(l.AccountCode):
			add(field, "The account code must be four digits.")
		case l.Debit < 0 || l.Credit < 0:
			add(field, "Amounts cannot be negative.")
		case l.Debit.IsPositive() == l.Credit.IsPositive():
			add(field, "A line is either a debit or a credit: exactly one must be above zero.")
		}
		var err error
		if debit, err = debit.Add(l.Debit); err != nil {
			overflow = true
		}
		if credit, err = credit.Add(l.Credit); err != nil {
			overflow = true
		}
	}
	if overflow {
		add("lines", "The totals are too large.")
	}

	if len(fields) > 0 {
		return apperr.Validation("ledger.entry_invalid", "The journal entry is not valid.", fields...)
	}
	if debit != credit {
		return apperr.Validation("ledger.unbalanced",
			fmt.Sprintf("Debits (%s) and credits (%s) must be equal.", debit, credit))
	}
	return nil
}

// reverseLines swaps the side of every line and keeps the amounts: the books after the original and
// the reversal are exactly as they were before either.
func reverseLines(lines []Line) []Line {
	out := make([]Line, len(lines))
	for i, l := range lines {
		out[i] = Line{AccountCode: l.AccountCode, Debit: l.Credit, Credit: l.Debit}
	}
	return out
}
