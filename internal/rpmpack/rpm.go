// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package rpmpack packs files to rpm files.
// It is designed to be simple to use and deploy, not requiring any filesystem access
// to create rpm files.
package rpmpack

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cavaliergopher/cpio"
	"github.com/klauspost/compress/zstd"
	gzip "github.com/klauspost/pgzip"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

var (
	// ErrWriteAfterClose is returned when a user calls Write() on a closed rpm.
	ErrWriteAfterClose = errors.New("rpm write after close")
	// ErrWrongFileOrder is returned when files are not sorted by name.
	ErrWrongFileOrder = errors.New("wrong file addition order")
)

// RPMMetaData contains meta info about the whole package.
type RPMMetaData struct {
	Name,
	Summary,
	Description,
	Version,
	Release,
	Arch,
	OS,
	Vendor,
	URL,
	Packager,
	Group,
	Licence,
	BuildHost,
	Compressor string
	Epoch     uint32
	BuildTime time.Time
	Provides,
	Obsoletes,
	Suggests,
	Recommends,
	Requires,
	Conflicts Relations
}

// RPM holds the state of a particular rpm file. Please use NewRPM to instantiate it.
type RPM struct {
	RPMMetaData
	di                *dirIndex
	payload           *bytes.Buffer
	payloadSize       uint
	cpio              *cpio.Writer
	basenames         []string
	dirindexes        []uint32
	filesizes         []uint32
	filemodes         []uint16
	fileowners        []string
	filegroups        []string
	filemtimes        []uint32
	filedigests       []string
	filelinktos       []string
	fileflags         []uint32
	closed            bool
	compressedPayload io.WriteCloser
	files             map[string]RPMFile
	prein             string
	postin            string
	preun             string
	postun            string
	pretrans          string
	posttrans         string
	customTags        map[int]IndexEntry
	customSigs        map[int]IndexEntry
	pgpSigner         func([]byte) ([]byte, error)
}

// NewRPM creates and returns a new RPM struct.
func NewRPM(m RPMMetaData) (*RPM, error) {
	var err error

	if m.OS == "" {
		m.OS = "linux"
	}

	if m.Arch == "" {
		m.Arch = "noarch"
	}

	p := &bytes.Buffer{}

	z, compressorName, err := setupCompressor(m.Compressor, p)
	if err != nil {
		return nil, err
	}

	// only use compressor name for the rpm tag, not the level
	m.Compressor = compressorName

	rpm := &RPM{
		RPMMetaData:       m,
		di:                newDirIndex(),
		payload:           p,
		compressedPayload: z,
		cpio:              cpio.NewWriter(z),
		files:             make(map[string]RPMFile),
		customTags:        make(map[int]IndexEntry),
		customSigs:        make(map[int]IndexEntry),
	}

	// A package must provide itself...
	rpm.Provides.addIfMissing(&Relation{
		Name:    rpm.Name,
		Version: rpm.FullVersion(),
		Sense:   SenseEqual,
	})

	return rpm, nil
}

func setupCompressor(compressorSetting string, w io.Writer) (wc io.WriteCloser,
	compressorType string, err error,
) {
	parts := strings.Split(compressorSetting, ":")
	if len(parts) > 2 {
		return nil, "", fmt.Errorf("malformed compressor setting: %s", compressorSetting)
	}

	compressorType = parts[0]
	compressorLevel := ""
	if len(parts) == 2 {
		compressorLevel = parts[1]
	}

	switch compressorType {
	case "":
		compressorType = "gzip"
		fallthrough
	case "gzip":
		level := 9

		if compressorLevel != "" {
			var err error

			level, err = strconv.Atoi(compressorLevel)
			if err != nil {
				return nil, "", fmt.Errorf("parse gzip compressor level: %w", err)
			}
		}

		wc, err = gzip.NewWriterLevel(w, level)
	case "lzma":
		if compressorLevel != "" {
			return nil, "", fmt.Errorf("no compressor level supported for lzma: %s", compressorLevel)
		}

		wc, err = lzma.NewWriter(w)
	case "xz":
		if compressorLevel != "" {
			return nil, "", fmt.Errorf("no compressor level supported for xz: %s", compressorLevel)
		}

		wc, err = xz.NewWriter(w)
	case "zstd":
		level := zstd.SpeedBetterCompression

		if compressorLevel != "" {
			var ok bool

			if intLevel, err := strconv.Atoi(compressorLevel); err == nil {
				level = zstd.EncoderLevelFromZstd(intLevel)
			} else {
				ok, level = zstd.EncoderLevelFromString(compressorLevel)
				if !ok {
					return nil, "", fmt.Errorf("invalid zstd compressor level: %s", compressorLevel)
				}
			}
		}

		wc, err = zstd.NewWriter(w, zstd.WithEncoderLevel(level))
	default:
		return nil, "", fmt.Errorf("unknown compressor type: %s", compressorType)
	}

	return wc, compressorType, err
}

// FullVersion properly combines version and release fields to a version string
func (r *RPM) FullVersion() string {
	if r.Release != "" {
		return fmt.Sprintf("%s-%s", r.Version, r.Release)
	}

	return r.Version
}

// AllowListDirs removes all directories which are not explicitly allowlisted.
func (r *RPM) AllowListDirs(allowList map[string]bool) {
	for fn, ff := range r.files {
		if ff.Mode&0o40000 == 0o40000 {
			if !allowList[fn] {
				delete(r.files, fn)
			}
		}
	}
}

// Write closes the rpm and writes the whole rpm to an io.Writer
func (r *RPM) Write(w io.Writer) error {
	if r.closed {
		return ErrWriteAfterClose
	}
	// Add all of the files, sorted alphabetically.
	fnames := []string{}
	for fn := range r.files {
		fnames = append(fnames, fn)
	}
	sort.Strings(fnames)
	for _, fn := range fnames {
		if err := r.writeFile(r.files[fn]); err != nil {
			return fmt.Errorf("failed to write file %q: %w", fn, err)
		}
	}
	if err := r.cpio.Close(); err != nil {
		return fmt.Errorf("failed to close cpio payload: %w", err)
	}
	if err := r.compressedPayload.Close(); err != nil {
		return fmt.Errorf("failed to close gzip payload: %w", err)
	}

	if _, err := w.Write(lead(r.Name, r.FullVersion())); err != nil {
		return fmt.Errorf("failed to write lead: %w", err)
	}
	// Write the regular header.
	h := newIndex(immutable)
	r.writeGenIndexes(h)

	// do not write file indexes if there are no files (meta package)
	// doing so will result in an invalid package
	if (len(r.files)) > 0 {
		r.writeFileIndexes(h)
	}

	if err := r.writeRelationIndexes(h); err != nil {
		return err
	}
	// CustomTags must be the last to be added, because they can overwrite values.
	h.AddEntries(r.customTags)
	hb, err := h.Bytes()
	if err != nil {
		return fmt.Errorf("failed to retrieve header: %w", err)
	}
	// Write the signatures
	s := newIndex(signatures)
	if err := r.writeSignatures(s, hb); err != nil {
		return fmt.Errorf("failed to create signatures: %w", err)
	}

	s.AddEntries(r.customSigs)
	sb, err := s.Bytes()
	if err != nil {
		return fmt.Errorf("failed to retrieve signatures header: %w", err)
	}

	if _, err := w.Write(sb); err != nil {
		return fmt.Errorf("failed to write signature bytes: %w", err)
	}
	// Signatures are padded to 8-byte boundaries
	if _, err := w.Write(make([]byte, (8-len(sb)%8)%8)); err != nil {
		return fmt.Errorf("failed to write signature padding: %w", err)
	}
	if _, err := w.Write(hb); err != nil {
		return fmt.Errorf("failed to write header body: %w", err)
	}
	if _, err := w.Write(r.payload.Bytes()); err != nil {
		return fmt.Errorf("failed to write payload: %w", err)
	}
	return nil
}

// SetPGPSigner registers a function that will accept the header and payload as bytes,
// and return a signature as bytes. The function should simulate what gpg does,
// probably by using golang.org/x/crypto/openpgp or by forking a gpg process.
func (r *RPM) SetPGPSigner(f func([]byte) ([]byte, error)) {
	r.pgpSigner = f
}

// Only call this after the payload and header were written.
func (r *RPM) writeSignatures(sigHeader *index, regHeader []byte) error {
	sigHeader.Add(sigSize, EntryInt32([]int32{int32(r.payload.Len() + len(regHeader))}))
	sigHeader.Add(sigSHA256, EntryString(fmt.Sprintf("%x", sha256.Sum256(regHeader))))
	sigHeader.Add(sigPayloadSize, EntryInt32([]int32{int32(r.payloadSize)}))
	if r.pgpSigner != nil {
		// For sha 256 you need to sign the header and payload separately
		header := append([]byte{}, regHeader...)
		headerSig, err := r.pgpSigner(header)
		if err != nil {
			return fmt.Errorf("call to signer failed: %w", err)
		}
		sigHeader.Add(sigRSA, EntryBytes(headerSig))

		body := append(header, r.payload.Bytes()...)
		bodySig, err := r.pgpSigner(body)
		if err != nil {
			return fmt.Errorf("call to signer failed: %w", err)
		}
		sigHeader.Add(sigPGP, EntryBytes(bodySig))
	}
	return nil
}

func (r *RPM) writeRelationIndexes(h *index) error {
	// add all relation categories
	if err := r.Provides.AddToIndex(h, tagProvides, tagProvideVersion, tagProvideFlags); err != nil {
		return fmt.Errorf("failed to add provides: %w", err)
	}
	if err := r.Obsoletes.AddToIndex(h, tagObsoletes, tagObsoleteVersion, tagObsoleteFlags); err != nil {
		return fmt.Errorf("failed to add obsoletes: %w", err)
	}
	if err := r.Suggests.AddToIndex(h, tagSuggests, tagSuggestVersion, tagSuggestFlags); err != nil {
		return fmt.Errorf("failed to add suggests: %w", err)
	}
	if err := r.Recommends.AddToIndex(h, tagRecommends, tagRecommendVersion, tagRecommendFlags); err != nil {
		return fmt.Errorf("failed to add recommends: %w", err)
	}
	if err := r.Requires.AddToIndex(h, tagRequires, tagRequireVersion, tagRequireFlags); err != nil {
		return fmt.Errorf("failed to add requires: %w", err)
	}
	if err := r.Conflicts.AddToIndex(h, tagConflicts, tagConflictVersion, tagConflictFlags); err != nil {
		return fmt.Errorf("failed to add conflicts: %w", err)
	}

	return nil
}

// AddCustomTag adds or overwrites a tag value in the index.
func (r *RPM) AddCustomTag(tag int, e IndexEntry) {
	r.customTags[tag] = e
}

// AddCustomSig adds or overwrites a signature tag value.
func (r *RPM) AddCustomSig(tag int, e IndexEntry) {
	r.customSigs[tag] = e
}

func (r *RPM) writeGenIndexes(h *index) {
	h.Add(tagHeaderI18NTable, EntryString("C"))
	h.Add(tagSize, EntryInt32([]int32{int32(r.payloadSize)}))
	h.Add(tagName, EntryString(r.Name))
	h.Add(tagVersion, EntryString(r.Version))
	h.Add(tagEpoch, EntryUint32([]uint32{r.Epoch}))
	h.Add(tagSummary, EntryString(r.Summary))
	h.Add(tagDescription, EntryString(r.Description))
	h.Add(tagBuildHost, EntryString(r.BuildHost))
	if !r.BuildTime.IsZero() {
		// time.Time zero value is confusing, avoid if not supplied
		// see https://github.com/google/rpmpack/issues/43
		h.Add(tagBuildTime, EntryInt32([]int32{int32(r.BuildTime.Unix())}))
	}
	h.Add(tagRelease, EntryString(r.Release))
	h.Add(tagPayloadFormat, EntryString("cpio"))
	h.Add(tagPayloadCompressor, EntryString(r.Compressor))
	h.Add(tagPayloadFlags, EntryString("9"))
	h.Add(tagArch, EntryString(r.Arch))
	h.Add(tagOS, EntryString(r.OS))
	h.Add(tagVendor, EntryString(r.Vendor))
	h.Add(tagLicence, EntryString(r.Licence))
	h.Add(tagPackager, EntryString(r.Packager))
	h.Add(tagGroup, EntryString(r.Group))
	h.Add(tagURL, EntryString(r.URL))
	h.Add(tagPayloadDigest, EntryStringSlice([]string{fmt.Sprintf("%x", sha256.Sum256(r.payload.Bytes()))}))
	h.Add(tagPayloadDigestAlgo, EntryInt32([]int32{hashAlgoSHA256}))

	// rpm utilities look for the sourcerpm tag to deduce if this is not a source rpm (if it has a sourcerpm,
	// it is NOT a source rpm).
	h.Add(tagSourceRPM, EntryString(fmt.Sprintf("%s-%s.src.rpm", r.Name, r.FullVersion())))
	if r.pretrans != "" {
		h.Add(tagPretrans, EntryString(r.pretrans))
		h.Add(tagPretransProg, EntryString("/bin/sh"))
	}
	if r.prein != "" {
		h.Add(tagPrein, EntryString(r.prein))
		h.Add(tagPreinProg, EntryString("/bin/sh"))
	}
	if r.postin != "" {
		h.Add(tagPostin, EntryString(r.postin))
		h.Add(tagPostinProg, EntryString("/bin/sh"))
	}
	if r.preun != "" {
		h.Add(tagPreun, EntryString(r.preun))
		h.Add(tagPreunProg, EntryString("/bin/sh"))
	}
	if r.postun != "" {
		h.Add(tagPostun, EntryString(r.postun))
		h.Add(tagPostunProg, EntryString("/bin/sh"))
	}
	if r.posttrans != "" {
		h.Add(tagPosttrans, EntryString(r.posttrans))
		h.Add(tagPosttransProg, EntryString("/bin/sh"))
	}
}

// WriteFileIndexes writes file related index headers to the header
func (r *RPM) writeFileIndexes(h *index) {
	h.Add(tagBasenames, EntryStringSlice(r.basenames))
	h.Add(tagDirindexes, EntryUint32(r.dirindexes))
	h.Add(tagDirnames, EntryStringSlice(r.di.AllDirs()))
	h.Add(tagFileSizes, EntryUint32(r.filesizes))
	h.Add(tagFileModes, EntryUint16(r.filemodes))
	h.Add(tagFileUserName, EntryStringSlice(r.fileowners))
	h.Add(tagFileGroupName, EntryStringSlice(r.filegroups))
	h.Add(tagFileMTimes, EntryUint32(r.filemtimes))
	h.Add(tagFileDigests, EntryStringSlice(r.filedigests))
	h.Add(tagFileLinkTos, EntryStringSlice(r.filelinktos))
	h.Add(tagFileFlags, EntryUint32(r.fileflags))

	inodes := make([]int32, len(r.dirindexes))
	digestAlgo := make([]int32, len(r.dirindexes))
	verifyFlags := make([]int32, len(r.dirindexes))
	fileRDevs := make([]int16, len(r.dirindexes))
	fileLangs := make([]string, len(r.dirindexes))

	for ii := range inodes {
		// is inodes just a range from 1..len(dirindexes)? maybe different with hard links
		inodes[ii] = int32(ii + 1)
		digestAlgo[ii] = hashAlgoSHA256
		// With regular files, it seems like we can always enable all of the verify flags
		verifyFlags[ii] = int32(-1)
		fileRDevs[ii] = int16(1)
	}
	h.Add(tagFileINodes, EntryInt32(inodes))
	h.Add(tagFileDigestAlgo, EntryInt32(digestAlgo))
	h.Add(tagFileVerifyFlags, EntryInt32(verifyFlags))
	h.Add(tagFileRDevs, EntryInt16(fileRDevs))
	h.Add(tagFileLangs, EntryStringSlice(fileLangs))
}

// AddPretrans adds a pretrans scriptlet
func (r *RPM) AddPretrans(s string) {
	r.pretrans = s
}

// AddPrein adds a prein scriptlet
func (r *RPM) AddPrein(s string) {
	r.prein = s
}

// AddPostin adds a postin scriptlet
func (r *RPM) AddPostin(s string) {
	r.postin = s
}

// AddPreun adds a preun scriptlet
func (r *RPM) AddPreun(s string) {
	r.preun = s
}

// AddPostun adds a postun scriptlet
func (r *RPM) AddPostun(s string) {
	r.postun = s
}

// AddPosttrans adds a posttrans scriptlet
func (r *RPM) AddPosttrans(s string) {
	r.posttrans = s
}

// AddFile adds an RPMFile to an existing rpm.
func (r *RPM) AddFile(f RPMFile) {
	if f.Name == "/" { // rpm does not allow the root dir to be included.
		return
	}
	r.files[f.Name] = f
}

// writeFile writes the file to the indexes and cpio.
func (r *RPM) writeFile(f RPMFile) error {
	dir, file := path.Split(f.Name)
	r.dirindexes = append(r.dirindexes, r.di.Get(dir))
	r.basenames = append(r.basenames, file)
	r.fileowners = append(r.fileowners, f.Owner)
	r.filegroups = append(r.filegroups, f.Group)
	r.filemtimes = append(r.filemtimes, f.MTime)
	r.fileflags = append(r.fileflags, uint32(f.Type))

	links := 1
	switch {
	case f.Mode&0o40000 != 0: // directory
		r.filesizes = append(r.filesizes, 4096)
		r.filedigests = append(r.filedigests, "")
		r.filelinktos = append(r.filelinktos, "")
		links = 2
	case f.Mode&0o120000 == 0o120000: //  symlink
		r.filesizes = append(r.filesizes, uint32(len(f.Body)))
		r.filedigests = append(r.filedigests, "")
		r.filelinktos = append(r.filelinktos, string(f.Body))
	default: // regular file
		f.Mode = f.Mode | 0o100000
		r.filesizes = append(r.filesizes, uint32(len(f.Body)))
		r.filedigests = append(r.filedigests, fmt.Sprintf("%x", sha256.Sum256(f.Body)))
		r.filelinktos = append(r.filelinktos, "")
	}
	r.filemodes = append(r.filemodes, uint16(f.Mode))

	// Ghost files have no payload
	if f.Type == GhostFile {
		return nil
	}
	return r.writePayload(f, links)
}

func (r *RPM) writePayload(f RPMFile, links int) error {
	hdr := &cpio.Header{
		Name:  f.Name,
		Mode:  cpio.FileMode(f.Mode),
		Size:  int64(len(f.Body)),
		Links: links,
	}
	if err := r.cpio.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to write payload file header: %w", err)
	}
	if _, err := r.cpio.Write(f.Body); err != nil {
		return fmt.Errorf("failed to write payload file content: %w", err)
	}
	r.payloadSize += uint(len(f.Body))
	return nil
}
