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
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
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
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
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

func main() {
	if err := runit("foo", "../alpine/user.rsa", "", os.Args[1]); err != nil {
		log.Fatalln(err)
	}
}

func runit(pathToFiles string, pathToKey string, workDir string, target string) (err error) {
	signatureTgz, err := os.Create(path.Join(workDir, "apk_signatures.tgz"))
	if err != nil {
		return err
	}
	defer signatureTgz.Close()

	controlTgz, err := os.Create(path.Join(workDir, "apk_control.tgz"))
	if err != nil {
		return err
	}
	defer controlTgz.Close()

	dataTgz, err := os.Create(path.Join(workDir, "apk_data.tgz"))
	if err != nil {
		return err
	}
	defer dataTgz.Close()

	size := int64(0)

	// create the data tgz
	log.Printf("before create data tgz")
	dataDigest, err := writeTgz(dataTgz, TarFull, func(tw *tar.Writer) error {
		log.Printf("dataTgz: %+v, TarFull: %+v", dataTgz, TarFull)

		err := filepath.Walk(pathToFiles, func(path string, info os.FileInfo, err error) error {
			log.Printf("path: %s, info: %+v, err: %+v", path, info, err)
			if err != nil {
				log.Print(err)
				//return nil
				return err
			}

			header, err := tar.FileInfoHeader(info, path)
			header.Name = path

			if info.IsDir() {
				fmt.Println("dir :", path)

				err := writeDir(tw, header)
				if err != nil {
					return err
				}
			} else {
				fmt.Println("file:", path)

				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer file.Close()

				err = writeFile(tw, header, file, true)
				if err != nil {
					return err
				}

				size += info.Size()
			}

			return nil
		})

		return err
	}, sha256.New())
	if err != nil {
		return err
	}
	fmt.Println("size       :", size)
	fmt.Println("data sha256:", hex.EncodeToString(dataDigest))

	// create the control tgz
	controlDigest, err := writeTgz(controlTgz, TarCut, func(tw *tar.Writer) error {
		infoContent := fmt.Sprintf(`
# Generated by abuild 3.2.0_rc1-r1
# using fakeroot version 1.22
# Tue May 15 20:25:33 UTC 2018
pkgname = %s
pkgver = %s
pkgdesc = foo
url = https://vafer.org
arch = x86_64
size = %d
datahash = %s
		`,
			"xbase",
			"1.0.0-r1",
			size,
			hex.EncodeToString(dataDigest),
		)

		infoHeader := &tar.Header{
			Name: ".PKGINFO",
			Mode: 0600,
			Size: int64(len(infoContent)),
		}

		err := writeFile(tw, infoHeader, strings.NewReader(infoContent), false)
		if err != nil {
			return err
		}

		return nil
	}, sha1.New())
	if err != nil {
		return err
	}
	fmt.Println("data sha1  :", hex.EncodeToString(controlDigest))

	//pemBytes, err := ioutil.ReadFile("../alpine/user.rsa")
	//pemBytes, err := ioutil.ReadFile("/home/appuser/.ssh/id_rsa")
	pemBytes, err := ioutil.ReadFile(pathToKey)
	if err != nil {
		return err
	}
	priv, err := ParseRsaPrivateKeyFromPemStr(string(pemBytes))
	if err != nil {
		return err
	}
	signed, err := priv.Sign(rand.Reader, controlDigest, crypto.SHA1)
	if err != nil {
		return err
	}
	fmt.Println("data sign  :", hex.EncodeToString(signed))

	// create the signature tgz
	_, err = writeTgz(signatureTgz, TarCut, func(tw *tar.Writer) error {
		keyname := "alpine-devel@lists.alpinelinux.org-4a6a0840"
		// needs to exist on the machine: /etc/apk/keys/<keyname>.rsa.pub

		signContent := signed
		signHeader := &tar.Header{
			Name: fmt.Sprintf(".SIGN.RSA.%s.rsa.pub", keyname),
			Mode: 0600,
			Size: int64(len(signContent)),
		}

		err := writeFile(tw, signHeader, bytes.NewReader(signContent), false)
		if err != nil {
			return err
		}

		return nil
	}, sha1.New())

	// combine
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer file.Close()

	tgzs := []*os.File{signatureTgz, controlTgz, dataTgz}

	for _, tgz := range tgzs {
		_, err = tgz.Seek(0, 0)
		if err != nil {
			return err
		}
		_, err = io.Copy(file, tgz)
		if err != nil {
			return err
		}
	}

	return err
}
