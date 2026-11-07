// Package ledger is the double-entry core (DEC-016, ADR-0004, ADR-0014). Money-moving code in other
// modules posts balanced journal entries here, inside its own transaction. Nothing outside this
// package writes journal rows, and no journal row is ever updated or deleted: a correction is a
// reversing entry.
package ledger

// System account codes. The chart is fixed in this slice; the same list is created per school by
// the EnsureSystemAccounts query. A test keeps the two in step.
const (
	AccountCashInHand       = "1000" // asset
	AccountBank             = "1100" // asset
	AccountFeeReceivable    = "1200" // asset: billed but not yet paid
	AccountAdvanceFees      = "2000" // liability: money received before it is billed (DEC-010)
	AccountSecurityDeposits = "2100" // liability, never income (BR-FEE-004)
	AccountFeeIncome        = "4000" // income
	AccountDiscountsGiven   = "5000" // expense
)

// SystemAccountCodes lists every built-in account code.
var SystemAccountCodes = []string{
	AccountCashInHand, AccountBank, AccountFeeReceivable, AccountAdvanceFees,
	AccountSecurityDeposits, AccountFeeIncome, AccountDiscountsGiven,
}
