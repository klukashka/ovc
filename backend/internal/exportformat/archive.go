package exportformat

import (
	"archive/zip"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"time"
)

var ErrTooLarge = errors.New("the archive passed its size bound")

type Archive struct {
	output   *boundedWriter
	zip      *zip.Writer
	modified time.Time
	entries  map[string]struct{}
}

type boundedWriter struct {
	destination io.Writer
	bound       int64
	written     int64
}

func (writer *boundedWriter) Write(data []byte) (int, error) {
	if writer.written+int64(len(data)) > writer.bound {
		return 0, ErrTooLarge
	}
	count, err := writer.destination.Write(data)
	writer.written += int64(count)
	if err != nil {
		return count, fmt.Errorf("write the archive: %w", err)
	}
	return count, nil
}

func NewArchive(destination io.Writer, bound int64, modified time.Time) *Archive {
	output := &boundedWriter{destination: destination, bound: bound}
	archive := zip.NewWriter(output)
	archive.RegisterCompressor(zip.Deflate, fastDeflate)
	return &Archive{output: output, zip: archive, modified: modified.UTC(), entries: map[string]struct{}{}}
}

func fastDeflate(destination io.Writer) (io.WriteCloser, error) {
	compressor, err := flate.NewWriter(destination, flate.BestSpeed)
	if err != nil {
		return nil, fmt.Errorf("start the archive compressor: %w", err)
	}
	return compressor, nil
}

func (archive *Archive) Create(name string) (io.Writer, error) {
	if err := checkEntryName(name); err != nil {
		return nil, err
	}
	if _, taken := archive.entries[name]; taken {
		return nil, ErrDuplicateEntryName
	}
	archive.entries[name] = struct{}{}
	entry, err := archive.zip.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: archive.modified})
	if err != nil {
		return nil, fmt.Errorf("start an archive entry: %w", err)
	}
	return entry, nil
}

func (archive *Archive) Written() int64 {
	return archive.output.written
}

func (archive *Archive) Close() error {
	if err := archive.zip.Close(); err != nil {
		return fmt.Errorf("finish the archive: %w", err)
	}
	return nil
}
