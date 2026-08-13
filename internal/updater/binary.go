package updater

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

func validateExecutable(path, goos, goarch string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open downloaded executable: %w", err)
	}
	defer file.Close()

	switch goos {
	case "windows":
		return validatePortableExecutable(file, goarch)
	case "linux":
		return validateELF(file, goarch)
	case "darwin":
		return validateMachO(file, goarch)
	default:
		return fmt.Errorf("unsupported update platform %s/%s", goos, goarch)
	}
}

func validatePortableExecutable(reader io.ReaderAt, goarch string) error {
	dosHeader := make([]byte, 64)
	if _, err := reader.ReadAt(dosHeader, 0); err != nil {
		return fmt.Errorf("read DOS header: %w", err)
	}
	if !bytes.Equal(dosHeader[:2], []byte{'M', 'Z'}) {
		return fmt.Errorf("download is not a Windows executable")
	}
	headerOffset := int64(binary.LittleEndian.Uint32(dosHeader[0x3c:0x40]))
	if headerOffset < 64 || headerOffset > 16<<20 {
		return fmt.Errorf("invalid PE header offset")
	}
	peHeader := make([]byte, 6)
	if _, err := reader.ReadAt(peHeader, headerOffset); err != nil {
		return fmt.Errorf("read PE header: %w", err)
	}
	if !bytes.Equal(peHeader[:4], []byte{'P', 'E', 0, 0}) {
		return fmt.Errorf("invalid PE signature")
	}
	machine := binary.LittleEndian.Uint16(peHeader[4:6])
	expected := map[string]uint16{
		"386":   0x014c,
		"amd64": 0x8664,
		"arm64": 0xaa64,
	}[goarch]
	if expected == 0 || machine != expected {
		return fmt.Errorf("PE architecture mismatch: got 0x%04x for %s", machine, goarch)
	}
	return nil
}

func validateELF(reader io.ReaderAt, goarch string) error {
	header := make([]byte, 20)
	if _, err := reader.ReadAt(header, 0); err != nil {
		return fmt.Errorf("read ELF header: %w", err)
	}
	if !bytes.Equal(header[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		return fmt.Errorf("download is not an ELF executable")
	}
	var order binary.ByteOrder
	switch header[5] {
	case 1:
		order = binary.LittleEndian
	case 2:
		order = binary.BigEndian
	default:
		return fmt.Errorf("invalid ELF byte order")
	}
	machine := order.Uint16(header[18:20])
	expected := map[string]uint16{
		"386":   3,
		"amd64": 62,
		"arm64": 183,
	}[goarch]
	if expected == 0 || machine != expected {
		return fmt.Errorf("ELF architecture mismatch: got %d for %s", machine, goarch)
	}
	return nil
}

func validateMachO(reader io.ReaderAt, goarch string) error {
	header := make([]byte, 8)
	if _, err := reader.ReadAt(header, 0); err != nil {
		return fmt.Errorf("read Mach-O header: %w", err)
	}
	var order binary.ByteOrder
	switch binary.BigEndian.Uint32(header[:4]) {
	case 0xfeedface, 0xfeedfacf:
		order = binary.BigEndian
	case 0xcefaedfe, 0xcffaedfe:
		order = binary.LittleEndian
	default:
		return fmt.Errorf("download is not a Mach-O executable")
	}
	cpu := order.Uint32(header[4:8])
	expected := map[string]uint32{
		"amd64": 0x01000007,
		"arm64": 0x0100000c,
	}[goarch]
	if expected == 0 || cpu != expected {
		return fmt.Errorf("Mach-O architecture mismatch: got 0x%08x for %s", cpu, goarch)
	}
	return nil
}
