//go:build windows

package manager

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DiskStats holds usage information for a filesystem.
type DiskStats struct {
	Path    string
	Total   uint64
	Used    uint64
	Free    uint64
	Percent float64
}

// recordingDir extracts the base directory from a filename pattern like
// "videos/{{.Username}}_{{.Year}}-..." → "videos".
func recordingDir(pattern string) string {
	idx := strings.Index(pattern, "{{")
	if idx == -1 {
		dir := filepath.Dir(pattern)
		if dir == "" || dir == "." {
			return "."
		}
		return filepath.Clean(dir)
	}
	dir := filepath.Dir(pattern[:idx])
	if dir == "" || dir == "." {
		return "."
	}
	return filepath.Clean(dir)
}

func getDiskStats(path string) (DiskStats, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return DiskStats{}, fmt.Errorf("abs path %s: %w", path, err)
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64

	pathPtr, err := windows.UTF16PtrFromString(absPath)
	if err != nil {
		return DiskStats{}, fmt.Errorf("utf16 path: %w", err)
	}

	kernel32 := windows.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")

	ret, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if ret == 0 {
		return DiskStats{}, fmt.Errorf("GetDiskFreeSpaceExW: %w", callErr)
	}

	used := totalNumberOfBytes - totalNumberOfFreeBytes
	var pct float64
	if totalNumberOfBytes > 0 {
		pct = float64(used) / float64(totalNumberOfBytes) * 100
	}
	return DiskStats{
		Path:    absPath,
		Total:   totalNumberOfBytes,
		Used:    used,
		Free:    totalNumberOfFreeBytes,
		Percent: pct,
	}, nil
}
