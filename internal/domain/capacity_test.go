package domain

import "testing"

// * [x] **P1_CORE_001** Strict decimal capacity parsing
// - Description: Parses valid whole-number decimal MB and GB values while rejecting ambiguous or malformed input.
// - Expected: Valid values produce exact bytes and every invalid value returns an error.
func TestParseCapacity(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    int64
		wantErr bool
	}{
		"megabytes":         {input: "700MB", want: 700_000_000},
		"gigabytes":         {input: "25GB", want: 25_000_000_000},
		"zero":              {input: "0GB", wantErr: true},
		"lowercase":         {input: "25gb", wantErr: true},
		"fraction":          {input: "1.5GB", wantErr: true},
		"binary unit":       {input: "1GiB", wantErr: true},
		"surrounding space": {input: " 1GB", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseCapacity(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("ParseCapacity() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCapacity() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParseCapacity() = %d, want %d", got, tc.want)
			}
		})
	}
}
