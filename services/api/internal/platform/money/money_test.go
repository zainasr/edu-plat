package money

import (
	"errors"
	"math"
	"testing"
)

// ADR-0004: money is integer paisa. Adding must never wrap around silently.
func TestADR_0004_AddDetectsOverflow(t *testing.T) {
	tests := []struct {
		name    string
		a, b    Money
		want    Money
		wantErr bool
	}{
		{name: "plain", a: 1500, b: 250, want: 1750},
		{name: "negative", a: 1000, b: -400, want: 600},
		{name: "zero", a: 0, b: 0, want: 0},
		{name: "positive overflow", a: math.MaxInt64, b: 1, wantErr: true},
		{name: "negative overflow", a: math.MinInt64, b: -1, wantErr: true},
		{name: "max plus zero", a: math.MaxInt64, b: 0, want: math.MaxInt64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.Add(tt.b)
			if tt.wantErr {
				if !errors.Is(err, ErrOverflow) {
					t.Fatalf("err = %v, want ErrOverflow", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %d, %v; want %d", got, err, tt.want)
			}
		})
	}
}

// The display form groups thousands and always shows two decimals: PKR 12,000.50.
func TestMoney_String(t *testing.T) {
	tests := []struct {
		in   Money
		want string
	}{
		{0, "PKR 0.00"},
		{5, "PKR 0.05"},
		{100, "PKR 1.00"},
		{1200050, "PKR 12,000.50"},
		{100000000, "PKR 1,000,000.00"},
		{-250, "PKR -2.50"},
		{math.MinInt64, "PKR -92,233,720,368,547,758.08"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("Money(%d).String() = %q, want %q", int64(tt.in), got, tt.want)
		}
	}
}

func TestMoney_Predicates(t *testing.T) {
	if !Money(0).IsZero() || Money(1).IsZero() {
		t.Fatal("IsZero is wrong")
	}
	if !Money(1).IsPositive() || Money(0).IsPositive() || Money(-1).IsPositive() {
		t.Fatal("IsPositive is wrong")
	}
}
