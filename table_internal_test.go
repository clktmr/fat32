package fat32

import (
	"bytes"
	"io"
	"os"
	"testing"
)

const (
	eoc    = uint32(0x0fffffff) // {0xff, 0xff, 0xff, 0x0f})
	eocMin = uint32(0x0ffffff8) // {0xf8, 0xff, 0xff, 0x0f})
)

func getValidFat32Table() *table {
	// make a duplicate, in case someone modifies what we return
	t := &table{}
	*t = *fsInfo.table
	// Flush the clusters cache first to persist modifications to the fakeClusters slice
	if err := t.clusters.Flush(); err != nil {
		panic(err)
	}
	// and because the clusters are copied by reference
	if fc, ok := t.clusters.reader.(fakeClusters); ok {
		cloned := make(fakeClusters, len(fc))
		copy(cloned, fc)
		t.clusters = newClusters(cloned, cloned, t.clusters.size)
	}

	return t
}

func TestFat32TableToBytes(t *testing.T) {
	t.Run("valid FAT32 table", func(t *testing.T) {
		table := getValidFat32Table()
		bReader := io.NewSectionReader(table, 0, int64(table.size))
		b, err := io.ReadAll(bReader)
		if err != nil {
			t.Fatalf("unable to read bytes from table: %v", err)
		}
		valid, err := os.ReadFile(Fat32File)
		if err != nil {
			t.Fatalf("error reading test fixture data from %s: %v", Fat32File, err)
		}
		validBytes := valid[fsInfo.firstFAT : fsInfo.firstFAT+fsInfo.sectorsPerFAT*fsInfo.bytesPerSector]
		if !bytes.Equal(validBytes, b) {
			t.Errorf("directory.toBytes() mismatched")
		}
	})
}

func TestFat32TableIsEoc(t *testing.T) {
	tests := []struct {
		cluster uint32
		eoc     bool
	}{
		{0xa7, false},
		{0x00, false},
		{0xFFFFFF7, false},
		{0xFFFFFF8, true},
		{0xFFFFFF9, true},
		{0xFFFFFFA, true},
		{0xFFFFFFB, true},
		{0xFFFFFFC, true},
		{0xFFFFFFD, true},
		{0xFFFFFFE, true},
		{0xFFFFFFF, true},
		{0xAFFFFFFF, true},
		{0x2FFFFFF8, true},
	}
	tab := table{}
	for _, tt := range tests {
		eoc := tab.isEoc(tt.cluster)
		if eoc != tt.eoc {
			t.Errorf("isEoc(%x): actual %t instead of expected %t", tt.cluster, eoc, tt.eoc)
		}
	}
}

type fakeClusters []byte

func (f fakeClusters) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= int64(len(f)) {
		return 0, io.EOF
	}
	n = copy(p, f[off:])
	if n < len(p) {
		err = io.EOF
	}
	return n, err
}

func (f fakeClusters) WriteAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= int64(len(f)) {
		return 0, io.ErrShortWrite
	}
	n = copy(f[off:], p)
	if n < len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func newFakeClusters(size int) *clusters {
	storage := make(fakeClusters, size)
	return newClusters(storage, storage, int64(size))
}
