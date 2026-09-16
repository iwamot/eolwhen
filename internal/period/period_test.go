package period

import (
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr string
	}{
		{"36h", 36 * time.Hour, ""},
		{"1d", 24 * time.Hour, ""},
		{"2w", 14 * 24 * time.Hour, ""},
		{"0d", 0, ""},
		{"1m", 0, "minutes or months"},
		{"1s", 0, `unknown unit "s"`},
		{"1", 0, "want a count"},
		{"", 0, "want a count"},
		{"d", 0, "want a count"},
		{"-1d", 0, "want a count"},
		{"+1d", 0, "want a count"},
		{"1.5d", 0, "want a count"},
		{"106751d", 106751 * 24 * time.Hour, ""},
		{"106752d", 0, "want a count"},
		{"15250w", 15250 * 7 * 24 * time.Hour, ""},
		{"15251w", 0, "want a count"},
		{"2562047h", 2562047 * time.Hour, ""},
		{"2562048h", 0, "want a count"},
		{"99999999999999999999d", 0, "want a count"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse err = %v", err)
			}
			if got != tt.want {
				t.Errorf("Parse = %v, want %v", got, tt.want)
			}
		})
	}
}
