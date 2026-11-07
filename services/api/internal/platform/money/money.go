// Package money is the one money type. An amount is a whole number of paisa (1/100 of a rupee)
// in an int64. There is no float anywhere near money (ADR-0004, data model §1). The currency is
// PKR; a per-tenant currency is a later setting.
package money

import (
	"errors"
	"fmt"
	"strings"
)

// Currency is the only currency in 1.0.
const Currency = "PKR"

// Money is an amount in paisa. Negative values are allowed (for example a variance); ledger lines
// use only non-negative amounts and say which side they are on.
type Money int64

// ErrOverflow is returned when an addition does not fit in an int64.
var ErrOverflow = errors.New("money: amount overflows int64")

// Add returns m+o, or ErrOverflow. The check is explicit because Go integers wrap silently, and a
// wrapped total would let an unbalanced entry look balanced.
func (m Money) Add(o Money) (Money, error) {
	sum := m + o
	if (o > 0 && sum < m) || (o < 0 && sum > m) {
		return 0, ErrOverflow
	}
	return sum, nil
}

// IsZero reports whether the amount is exactly zero.
func (m Money) IsZero() bool { return m == 0 }

// IsPositive reports whether the amount is greater than zero.
func (m Money) IsPositive() bool { return m > 0 }

// String formats for people, such as "PKR 12,000.50". It is for logs and messages, not for APIs:
// the API sends the integer paisa and the currency separately.
func (m Money) String() string {
	// Work in uint64 so the most negative int64 does not overflow when negated.
	neg := m < 0
	abs := uint64(m)
	if neg {
		abs = -abs
	}
	whole, frac := abs/100, abs%100
	digits := fmt.Sprintf("%d", whole)

	var grouped strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(d)
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s %s%s.%02d", Currency, sign, grouped.String(), frac)
}
