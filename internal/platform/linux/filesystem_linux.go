//go:build linux

package linux

import (
	"fmt"
	"syscall"
)

const (
	nfsSuperMagic   = 0x6969
	cifsSuperMagic  = 0xFF534D42
	smb2SuperMagic  = 0xFE534D42
	ninePSuperMagic = 0x01021997
)

// Classifier classifies Linux filesystem paths as local or network-backed.
type Classifier struct{}

// IsLocal reports whether path is not on a known network filesystem.
func (Classifier) IsLocal(path string) (bool, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false, fmt.Errorf("stat filesystem: %w", err)
	}
	return isLocalFilesystemType(stat.Type), nil
}

func isLocalFilesystemType(filesystemType int64) bool {
	switch filesystemType {
	case nfsSuperMagic, cifsSuperMagic, smb2SuperMagic, ninePSuperMagic:
		return false
	default:
		return true
	}
}
