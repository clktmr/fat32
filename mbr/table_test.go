package mbr_test

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/clktmr/fat32/mbr"
)

const (
	mbrFile = "./testdata/mbr.img"
	tenMB   = 10 * 1024 * 1024
)

var (
	intImage     = os.Getenv("TEST_IMAGE")
	keepTmpFiles = os.Getenv("KEEPTESTFILES") == ""
)

func tmpDisk(source string, size int64) (*os.File, error) {
	filename := "disk_test"
	f, err := os.CreateTemp("", filename)
	if err != nil {
		return nil, fmt.Errorf("Failed to create tempfile %s :%v", filename, err)
	}

	// either copy the contents of the source file over, or make a file of appropriate size
	if source == "" {
		// make it a 10MB file
		_ = f.Truncate(size)
	} else {
		b, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("Failed to read contents of %s: %v", source, err)
		}
		written, err := f.Write(b)
		if err != nil {
			return nil, fmt.Errorf("Failed to write contents of %s to %s: %v", source, filename, err)
		}
		if written != len(b) {
			return nil, fmt.Errorf("wrote only %d bytes of %s to %s instead of %d", written, source, filename, len(b))
		}
	}

	return f, nil
}

// compareMBRBytes compare bytes from 446:512
// need compare function because we ignore cylinder/head/sector geometry
func compareMBRBytes(b1, b2 []byte) bool {
	if (b1 == nil && b2 != nil) || (b2 == nil && b1 != nil) {
		return false
	}
	if b1 == nil && b2 == nil {
		return true
	}
	if len(b1) != 66 || len(b2) != 66 {
		return false
	}
	// need to compare each of the partition arrays
	if !mbr.PartitionEqualBytes(b1[0:16], b2[0:16]) {
		return false
	}
	if !mbr.PartitionEqualBytes(b1[16:32], b2[16:32]) {
		return false
	}
	if !mbr.PartitionEqualBytes(b1[32:48], b2[32:48]) {
		return false
	}
	if !mbr.PartitionEqualBytes(b1[48:64], b2[48:64]) {
		return false
	}
	if !bytes.Equal(b1[64:66], b2[64:66]) {
		return false
	}
	return true
}

func TestTableRead(t *testing.T) {
	t.Run("error reading file", func(t *testing.T) {
		expected := errors.New("error reading MBR from file")
		f := &mbr.FakeBackend{
			Reader: func(b []byte, offset int64) (int, error) {
				return 0, expected
			},
		}
		table, err := mbr.Read(f, 512, 512)
		if table != nil {
			t.Errorf("returned table instead of nil")
		}
		if err == nil {
			t.Errorf("returned nil error instead of actual errors")
		}
		if !errors.Is(err, expected) {
			t.Errorf("error value")
		}
	})
	t.Run("insufficient data read", func(t *testing.T) {
		size := 100
		f := &mbr.FakeBackend{
			Reader: func(b []byte, offset int64) (int, error) {
				return size, nil
			},
		}
		table, err := mbr.Read(f, 512, 512)
		if table != nil {
			t.Errorf("returned table instead of nil")
		}
		if err == nil {
			t.Errorf("returned nil error instead of actual errors")
		}
	})
	t.Run("successful read", func(t *testing.T) {
		f, err := os.Open(mbrFile)
		if err != nil {
			t.Fatalf("error opening file %s to read: %v", mbrFile, err)
		}
		defer f.Close()
		table, err := mbr.Read(f, 512, 512)
		if err != nil {
			t.Errorf("returned error %v instead of nil", err)
		}
		if table == nil {
			t.Errorf("returned nil instead of table")
		}
		expected := mbr.GetValidTable()
		if table == nil && expected != nil || !table.Equal(expected) {
			t.Errorf("actual table was %v instead of expected %v", table, expected)
		}
	})
}
func TestTableWrite(t *testing.T) {
	t.Run("error writing file", func(t *testing.T) {
		table := mbr.GetValidTable()
		expected := errors.New("error writing partition table to disk")
		f := &mbr.FakeBackend{
			Writer: func(b []byte, offset int64) (int, error) {
				return 0, expected
			},
		}
		err := table.Write(f, tenMB)
		if err == nil {
			t.Errorf("returned nil error instead of actual errors")
		}
		if !errors.Is(err, expected) {
			t.Errorf("error value")
		}
	})
	t.Run("insufficient data written", func(t *testing.T) {
		table := mbr.GetValidTable()
		var size int
		f := &mbr.FakeBackend{
			Writer: func(b []byte, offset int64) (int, error) {
				size = len(b) - 1
				return size, nil
			},
		}
		err := table.Write(f, tenMB)
		if err == nil {
			t.Errorf("returned nil error instead of actual errors")
		}
	})
	t.Run("successful write", func(t *testing.T) {
		table := mbr.GetValidTable()
		mbrFileHandle, err := os.Open(mbrFile)
		if err != nil {
			t.Fatalf("error opening file %s: %v", mbrFile, err)
		}
		defer mbrFileHandle.Close()
		mbrBytes := make([]byte, 512)
		read, err := mbrFileHandle.ReadAt(mbrBytes, 0)
		if err != nil {
			t.Fatalf("error reading MBR from file %s: %v", mbrFile, err)
		}
		if read != len(mbrBytes) {
			t.Fatalf("read %d instead of %d bytes MBR from file %s", read, len(mbrBytes), mbrFile)
		}
		bootloader := mbrBytes[:446]
		remainder := mbrBytes[446:]
		tableBytes := make([]byte, 0, len(remainder))

		f := &mbr.FakeBackend{
			Writer: func(b []byte, offset int64) (int, error) {
				switch offset {
				case 446:
					tableBytes = append(tableBytes, b...)
				default:
					t.Fatalf("Attempted to write at position %d instead of %d", offset, 446)
				}
				return len(b), nil
			},
		}
		err = table.Write(f, tenMB)
		if err != nil {
			t.Errorf("returned error %v instead of nil", err)
		}
		if !compareMBRBytes(remainder, tableBytes) {
			t.Log(remainder)
			t.Log(tableBytes)
			t.Errorf("mismatched MBR")
		}
		// need to check that bootloader was unchanged
		bootloaderBytes := make([]byte, 446)
		read, err = mbrFileHandle.ReadAt(bootloaderBytes, 0)
		if err != nil {
			t.Fatalf("error reading bootloader from file %s: %v", mbrFile, err)
		}
		if read != len(bootloaderBytes) {
			t.Fatalf("read %d instead of %d bytes bootloader from file %s", read, len(bootloaderBytes), mbrFile)
		}
		if !bytes.Equal(bootloader, bootloaderBytes) {
			t.Error("bootloader was changed when it should not be")
		}
	})
}
func TestGetPartitionSize(t *testing.T) {
	table := mbr.GetValidTable()
	maxPart := len(table.Partitions)
	request := maxPart - 1
	size := table.Partitions[request].GetSize()
	expected := table.Partitions[request].Size
	if size != int64(expected) {
		t.Errorf("received size %d instead of %d", size, expected)
	}
}
func TestGetPartitionStart(t *testing.T) {
	table := mbr.GetValidTable()
	maxPart := len(table.Partitions)
	request := maxPart - 1
	start := table.Partitions[request].GetStart()
	expected := table.Partitions[request].Start
	if start != int64(expected) {
		t.Errorf("received start %d instead of %d", start, expected)
	}
}
func TestReadPartitionContents(t *testing.T) {
	table := mbr.GetValidTable()
	maxPart := len(table.Partitions)
	request := maxPart - 1
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	size := 100
	b2 := make([]byte, size)
	_, _ = rand.Read(b2)
	f := &mbr.FakeBackend{
		Reader: func(b []byte, offset int64) (int, error) {
			copy(b, b2)
			return size, io.EOF
		},
	}
	read, err := table.Partitions[request].ReadContents(f, writer)
	if read != int64(size) {
		t.Errorf("returned %d bytes read instead of %d", read, size)
	}
	if err != nil {
		t.Errorf("error was not nil")
	}
	writer.Flush()
	if !bytes.Equal(b.Bytes(), b2) {
		t.Errorf("Mismatched bytes data")
		t.Log(b.Bytes())
		t.Log(b2)
	}
}
func TestWritePartitionContents(t *testing.T) {
	table := mbr.GetValidTable()
	request := 0
	size := table.Partitions[request].Size * uint32(table.LogicalSectorSize)
	b := make([]byte, size)
	_, _ = rand.Read(b)
	reader := bytes.NewReader(b)
	b2 := make([]byte, 0, size)
	f := &mbr.FakeBackend{
		Writer: func(b []byte, offset int64) (int, error) {
			b2 = append(b2, b...)
			return len(b), nil
		},
	}
	written, err := table.Partitions[request].WriteContents(f, reader)
	if written != uint64(size) {
		t.Errorf("returned %d bytes written instead of %d", written, size)
	}
	if err != nil {
		t.Errorf("error was not nil: %v", err)
	}
	if !bytes.Equal(b2, b) {
		t.Errorf("Bytes mismatch")
		t.Log(b)
		t.Log(b2)
	}
}
