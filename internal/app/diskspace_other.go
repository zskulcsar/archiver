//go:build !linux

package app

type defaultDiskSpaceChecker struct{}

func (defaultDiskSpaceChecker) AvailableBytes(string) (int64, error) {
	return int64(^uint64(0) >> 1), nil
}
