//go:build !windows

package automation

import (
	"path/filepath"
	"syscall"
)

// DiskSpaceInfo holds free and total capacity for a target path's filesystem.
type DiskSpaceInfo struct {
	Path    string
	FreeGB  float64
	TotalGB float64
	FreePct float64
}

// GetDiskSpace queries Unix Statfs cleanly without CGO.
func GetDiskSpace(path string) (DiskSpaceInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(abs, &st); err != nil {
		return DiskSpaceInfo{}, err
	}
	totalBytes := float64(st.Blocks) * float64(st.Bsize)
	availBytes := float64(st.Bavail) * float64(st.Bsize)
	if totalBytes <= 0 {
		return DiskSpaceInfo{Path: abs}, nil
	}
	const gb = 1024 * 1024 * 1024
	return DiskSpaceInfo{
		Path:    abs,
		FreeGB:  availBytes / gb,
		TotalGB: totalBytes / gb,
		FreePct: (availBytes / totalBytes) * 100.0,
	}, nil
}
