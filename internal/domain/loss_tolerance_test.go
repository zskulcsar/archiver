package domain

import "testing"

// * [x] **P1_CORE_006** Strict loss-tolerance parsing
// - Description: Accepts only whole percentages in the documented local PAR2 range.
// - Expected: Valid percentages parse and malformed or out-of-range values fail.
func TestParseLossTolerance(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    int
		wantErr bool
	}{
		"disabled": {input: "0%", want: 0},
		"default":  {input: "10%", want: 10},
		"maximum":  {input: "50%", want: 50},
		"bare":     {input: "10", wantErr: true},
		"fraction": {input: "1.5%", wantErr: true},
		"too high": {input: "51%", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseLossTolerance(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("ParseLossTolerance() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLossTolerance() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParseLossTolerance() = %d, want %d", got, tc.want)
			}
		})
	}
}
