//go:build linux

package linux

import (
	"testing"
)

// * [x] **P1_CORE_013** Linux local and network filesystem classification
// - Description: Distinguishes common Linux network filesystem types from local filesystem types.
// - Expected: NFS and CIFS are rejected while ext4 is accepted as local.
func TestIsLocalFilesystemType(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		filesystemType int64
		want           bool
	}{
		"ext4": {filesystemType: 0xEF53, want: true},
		"nfs":  {filesystemType: 0x6969, want: false},
		"cifs": {filesystemType: 0xFF534D42, want: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isLocalFilesystemType(test.filesystemType); got != test.want {
				t.Fatalf("isLocalFilesystemType(%#x) = %t, want %t", test.filesystemType, got, test.want)
			}
		})
	}
}
