/*
Copyright 2019 Torsten Curdt

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package apk

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"hash"
	"io"
	"sync/atomic"
	"time"
)

type WriterCounter struct {
	io.Writer
	count  uint64
	writer io.Writer
}

func NewWriterCounter(w io.Writer) *WriterCounter {
	return &WriterCounter{
		writer: w,
	}
}

func (counter *WriterCounter) Write(buf []byte) (int, error) {
	n, err := counter.writer.Write(buf)
	atomic.AddUint64(&counter.count, uint64(n))
	return n, err
}

func (counter *WriterCounter) Count() uint64 {
	return atomic.LoadUint64(&counter.count)
}

func writeDir(tw *tar.Writer, header *tar.Header) error {
	header.ChangeTime = time.Time{}
	header.AccessTime = time.Time{}
	header.Format = tar.FormatUSTAR

	err := tw.WriteHeader(header)
	if err != nil {
		return err
	}

	return nil
}

func writeFile(tw *tar.Writer, header *tar.Header, file io.Reader, hash bool) error {
	// if hash {
	// 	header.Format = tar.FormatPAX

	// 	digest := sha1.New()
	// 	_, err := io.Copy(digest, file)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	header.PAXRecords = map[string]string{
	// 		"APK-TOOLS.checksum.SHA1": "f572d396fae9206628714fb2ce00f72e94f2258f",
	// 		// "APK-TOOLS.checksum.SHA1": hex.EncodeToString(digest.Sum(nil)),
	// 	}
	// } else {
	header.Format = tar.FormatUSTAR
	header.ChangeTime = time.Time{}
	header.AccessTime = time.Time{}
	// }

	err := tw.WriteHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	if err != nil {
		return err
	}

	return nil
}

type TarKind int

const (
	TarFull TarKind = iota
	TarCut
)

func writeTgz(w io.Writer, kind TarKind, builder func(tw *tar.Writer) error, digest hash.Hash) ([]byte, error) {
	mw := io.MultiWriter(digest, w)
	gw := gzip.NewWriter(mw)
	cw := NewWriterCounter(gw)
	bw := bufio.NewWriterSize(cw, 4096)
	tw := tar.NewWriter(bw)

	err := builder(tw)
	if err != nil {
		return nil, err
	}

	// handle the cut vs full tars
	bw.Flush()
	tw.Close()
	if kind == TarFull {
		bw.Flush()
	}

	size := cw.Count()
	alignedSize := (size + 511) & ^uint64(511)

	increase := alignedSize - size
	if increase > 0 {
		b := make([]byte, increase, increase)
		_, err = cw.Write(b)
		if err != nil {
			return nil, err
		}
	}

	gw.Close()

	return digest.Sum(nil), nil
}

func ParseRsaPrivateKeyFromPemStr(privPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the key")
	}

	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return priv, nil
}
