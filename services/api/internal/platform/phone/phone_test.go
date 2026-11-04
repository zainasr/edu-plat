package phone

import (
	"errors"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "local format", in: "03001234567", want: "+923001234567"},
		{name: "international", in: "+923001234567", want: "+923001234567"},
		{name: "spaces and dashes", in: " 0300-123 4567 ", want: "+923001234567"},
		{name: "too short", in: "0300123456", wantErr: true},
		{name: "not a mobile prefix", in: "02112345678", wantErr: true},
		{name: "letters", in: "0300abc4567", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Normalize(%q) err = %v, want ErrInvalid", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMask(t *testing.T) {
	if got := Mask("+923001234567"); got != "+92300****567" {
		t.Fatalf("Mask = %q, want +92300****567", got)
	}
	if got := Mask("bad"); got != "****" {
		t.Fatalf("Mask(short) = %q, want ****", got)
	}
}
