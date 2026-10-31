package msi

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"

	"go.digitalxero.dev/go-msi"
)

// shortcutIcon rejects raw ICOs: advertised shortcuts require an EXE-format
// stream, even when the installed target has a different extension.
// https://learn.microsoft.com/en-us/windows/win32/msi/icon-table
func shortcutIcon(filename string) (msi.FileSource, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	image, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("expected a PE image containing icon resources (raw .ico files are not supported): %w", err)
	}
	defer image.Close()

	var resources pe.DataDirectory
	switch header := image.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		resources = header.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE]
	case *pe.OptionalHeader64:
		resources = header.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE]
	}
	for _, section := range image.Sections {
		if resources.VirtualAddress < section.VirtualAddress {
			continue
		}
		offset := uint64(resources.VirtualAddress - section.VirtualAddress)
		if resources.Size < 16 || offset+uint64(resources.Size) > uint64(section.Size) {
			continue
		}
		root := make([]byte, 16)
		if _, err := section.ReadAt(root, int64(offset)); err != nil {
			return nil, fmt.Errorf("reading icon resource directory: %w", err)
		}
		count := uint64(binary.LittleEndian.Uint16(root[12:])) + uint64(binary.LittleEndian.Uint16(root[14:]))
		if 16+count*8 > uint64(resources.Size) {
			break
		}
		entries := make([]byte, count*8)
		if _, err := section.ReadAt(entries, int64(offset+16)); err != nil {
			return nil, fmt.Errorf("reading icon resource entries: %w", err)
		}
		var icon, group bool
		for i := 0; i < len(entries); i += 8 {
			id := binary.LittleEndian.Uint32(entries[i:])
			subdirectory := binary.LittleEndian.Uint32(entries[i+4:])
			if subdirectory&0x80000000 == 0 || uint64(subdirectory&0x7fffffff)+16 > uint64(resources.Size) {
				continue
			}
			icon = icon || id == 3    // RT_ICON
			group = group || id == 14 // RT_GROUP_ICON
		}
		if icon && group {
			return msi.FileSourceFromBytes(data), nil
		}
		break
	}
	return nil, fmt.Errorf("expected a PE image containing icon resources (RT_ICON and RT_GROUP_ICON)")
}
