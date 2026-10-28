package msi_test

import (
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// TestIconResourcesWindows asks the real Windows shell to extract the icon
// from the same PE payload round-tripped through the MSI Icon table tests.
func TestIconResourcesWindows(t *testing.T) {
	filename, err := filepath.Abs("../testdata/msi/icon.dll")
	require.NoError(t, err)
	name, err := windows.UTF16PtrFromString(filename)
	require.NoError(t, err)
	shell := windows.NewLazySystemDLL("shell32.dll")
	extract := shell.NewProc("ExtractIconExW")
	destroy := windows.NewLazySystemDLL("user32.dll").NewProc("DestroyIcon")
	groups, _, _ := extract.Call(uintptr(unsafe.Pointer(name)), ^uintptr(0), 0, 0, 0)
	require.Equal(t, uintptr(1), groups, "the fixture must contain one RT_GROUP_ICON resource")
	var large, small windows.Handle
	count, _, _ := extract.Call(uintptr(unsafe.Pointer(name)), 0,
		uintptr(unsafe.Pointer(&large)), uintptr(unsafe.Pointer(&small)), 1)
	defer func() {
		if large != 0 {
			destroy.Call(uintptr(large))
		}
		if small != 0 {
			destroy.Call(uintptr(small))
		}
	}()
	// The return value counts extracted handles, including both requested
	// sizes; it is not the number of RT_GROUP_ICON resources.
	require.Equal(t, uintptr(2), count, "Windows must decode both large and small icons from the PE resources")
	require.NotZero(t, large)
	require.NotZero(t, small)
}
