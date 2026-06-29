package fat32

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	cacheSize = 4096
	cacheMask = cacheSize - 1
)

// table a FAT32 table
type table struct {
	fatID          uint32
	eocMarker      uint32
	unusedMarker   uint32
	clusters       *clusters
	rootDirCluster uint32
	size           uint32
	maxCluster     uint32
}

type clusters struct {
	reader io.ReaderAt
	writer io.WriterAt // optional
	size   int64

	cache      [cacheSize]byte
	cacheStart int64 // -1 if no block is loaded
	dirty      bool
}

func (c *clusters) load(alignStart int64) error {
	if c.cacheStart != alignStart {
		if err := c.Flush(); err != nil {
			return err
		}

		readSize := min(cacheSize, c.size-alignStart)
		if readSize > 0 {
			_, err := c.reader.ReadAt(c.cache[:readSize], alignStart)
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				return err
			}
		}
		c.cacheStart = alignStart
	}
	return nil
}

func (c *clusters) Cluster(i uint32) (uint32, error) {
	off := int64(i) << 2
	alignStart := off &^ cacheMask

	if err := c.load(alignStart); err != nil {
		return 0, err
	}

	val := binary.LittleEndian.Uint32(c.cache[off&cacheMask : (off&cacheMask)+4])
	return uint32(val), nil
}

func (c *clusters) SetCluster(i int, val int32) error {
	if c.writer == nil {
		return errors.New("clusters writer is nil")
	}

	off := int64(i) << 2
	alignStart := off &^ cacheMask

	if err := c.load(alignStart); err != nil {
		return err
	}

	binary.LittleEndian.PutUint32(c.cache[off&cacheMask:(off&cacheMask)+4], uint32(val))
	c.dirty = true
	return nil
}

func (c *clusters) Flush() error {
	if c.dirty {
		if c.writer == nil {
			return ErrReadonlyFilesystem
		}
		if c.cacheStart != -1 {
			writeSize := min(cacheSize, c.size-c.cacheStart)
			if writeSize > 0 {
				_, err := c.writer.WriteAt(c.cache[:writeSize], c.cacheStart)
				if err != nil {
					return err
				}
			}
		}
		c.dirty = false
	}
	return nil
}

func newClusters(r io.ReaderAt, w io.WriterAt, size int64) *clusters {
	return &clusters{
		reader:     r,
		writer:     w,
		size:       size,
		cacheStart: -1,
	}
}

func (c *clusters) Equal(other *clusters) (bool, error) {
	if c.size != other.size {
		return false, nil
	}
	numClusters := uint32(c.size >> 2)
	for i := uint32(2); i < numClusters; i++ {
		a, err := c.Cluster(i)
		if err != nil {
			return false, err
		}
		b, err := other.Cluster(i)
		if err != nil {
			return false, err
		}
		if a != b {
			return false, nil
		}
	}
	return true, nil
}

/*
  when reading from disk, remember that *any* of the following is a valid eocMarker:
  0x?ffffff8 - 0x?fffffff
*/

func (t *table) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(t.size) {
		return 0, io.EOF
	}

	n := 0
	for n < len(p) && off < int64(t.size) {
		if off < 4 {
			shift := uint(off << 3)
			p[n] = byte(t.fatID >> shift)
			n++
			off++
		} else if off < 8 {
			shift := uint((off - 4) << 3)
			p[n] = byte(t.eocMarker >> shift)
			n++
			off++
		} else {
			clusterIdx := uint32(off >> 2)
			cVal, err := t.clusters.Cluster(clusterIdx)
			if err != nil {
				return n, err
			}
			byteIdx := off & 3

			for byteIdx < 4 && n < len(p) && off < int64(t.size) {
				p[n] = byte(cVal >> uint(byteIdx<<3))
				n++
				off++
				byteIdx++
			}
		}
	}

	var err error
	if off >= int64(t.size) {
		err = io.EOF
	}
	return n, err
}

func (t *table) isEoc(cluster uint32) bool {
	return cluster&0xFFFFFF8 == 0xFFFFFF8
}
