//go:build windows

package automation

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	modKernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpace = modKernel32.NewProc("GetDiskFreeSpaceExW")
)

// DiskSpaceInfo holds free and total capacity for a target path's filesystem.
type DiskSpaceInfo struct {
	Path    string
	FreeGB  float64
	TotalGB float64
	FreePct float64
}

// GetDiskSpace queries Windows GetDiskFreeSpaceExW cleanly without CGO.
func GetDiskSpace(path string) (DiskSpaceInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	ptr, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return DiskSpaceInfo{}, err
	}
	var freeAvail, totalBytes, totalFree uint64
	r1, _, callErr := procGetDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&freeAvail)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if r1 == 0 {
		return DiskSpaceInfo{}, callErr
	}
	if totalBytes == 0 {
		return DiskSpaceInfo{Path: abs}, nil
	}
	const gb = 1024 * 1024 * 1024
	return DiskSpaceInfo{
		Path:    abs,
		FreeGB:  float64(freeAvail) / gb,
		TotalGB: float64(totalBytes) / gb,
		FreePct: (float64(freeAvail) / float64(totalBytes)) * 100.0,
	}, nil
}
