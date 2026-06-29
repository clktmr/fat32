package fat32

import "io"

type zeroReader struct{}

func (r zeroReader) Read(p []byte) (n int, err error) { clear(p); return len(p), nil }

type multiWriterAt struct {
	writers []io.WriterAt
}

func newMultiWriterAt(writers ...io.WriterAt) io.WriterAt {
	return &multiWriterAt{writers: writers}
}

func (m *multiWriterAt) WriteAt(p []byte, off int64) (n int, err error) {
	for _, w := range m.writers {
		n, err = w.WriteAt(p, off)
		if err != nil {
			return n, err
		}
	}
	return len(p), nil
}
