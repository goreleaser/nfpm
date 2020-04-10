package apk

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goreleaser/nfpm"

	"github.com/stretchr/testify/assert"
)

// nolint: gochecknoglobals
var updateApk = flag.Bool("update-apk", false, "update apk .golden files")

func getPrivateKeyFile() string {
	return path.Join("testdata", "keyfile", "id_rsa")
}
func getBase64PrivateKey(t *testing.T) string {
	key, err := fileToBase64String(getPrivateKeyFile())
	assert.NoError(t, err)
	return key
}

func TestDefaultWithNFPMInfo(t *testing.T) {
	workDir := path.Join("testdata", "workdir")
	tempDir, err := ioutil.TempDir(workDir, "test-run")
	if err != nil {
		log.Fatal(err)
	}
	skipVerifyInfo := isSkipVerifyInfo()
	defer func() {
		if !skipVerifyInfo {
			// cleanup temp files
			assert.Nil(t, os.RemoveAll(tempDir))
		}
	}()

	apkFileToCreate, err := os.Create(path.Join(tempDir, "apkToCreate.apk"))
	assert.NoError(t, err)
	if skipVerifyInfo {
		t.Log("apk at", tempDir)
	}

	assert.NoError(t, Default.Package(&nfpm.Info{
		Name:             "foo",
		PrivateKeyBase64: getBase64PrivateKey(t),
		Overridables: nfpm.Overridables{
			Files: map[string]string{
				path.Join("testdata", "files", "control.golden"): "/testdata/files/control.golden",
			},
			EmptyFolders: []string{
				"/testdata/files/emptydir",
			},
		},
	}, apkFileToCreate))

	if !skipVerifyInfo {
		// @todo replace or remove .apk file size assertions
		verifyFileSizeRange(t, apkFileToCreate, 1275, 1411)
	}
}

func isSkipVerifyInfo() bool {
	skipVerifyInfo, _ := strconv.ParseBool(os.Getenv("skipVerifyInfo"))
	return skipVerifyInfo
}

func verifyFileSizeRange(t *testing.T, fileToVerify *os.File, expectedSizeMin, expectedSizeMax int64) {
	fi, err := fileToVerify.Stat()
	assert.Nil(t, err)
	assert.True(t, (expectedSizeMin <= fi.Size()) && (fi.Size() <= expectedSizeMax),
		"bad value range: expectedSizeMin: %d, expectedSizeMax: %d, actual: %d, file: %s", expectedSizeMin, expectedSizeMax, fi.Size(), fileToVerify) // yuck
}

func exampleInfo(t *testing.T) *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:             "foo",
		Arch:             "amd64",
		Description:      "Foo does things",
		Priority:         "extra",
		Maintainer:       "Carlos A Becker <pkg@carlosbecker.com>",
		Version:          "v1.0.0",
		Release:          "r1",
		Section:          "default",
		Homepage:         "http://carlosbecker.com",
		Vendor:           "nope",
		PrivateKeyBase64: getBase64PrivateKey(t),
		Overridables: nfpm.Overridables{
			Depends: []string{
				"bash",
			},
			Recommends: []string{
				"git",
			},
			Suggests: []string{
				"bash",
			},
			Replaces: []string{
				"svn",
			},
			Provides: []string{
				"bzr",
			},
			Conflicts: []string{
				"zsh",
			},
			Files: map[string]string{
				"../testdata/fake":          "/usr/local/bin/fake",
				"../testdata/whatever.conf": "/usr/share/doc/fake/fake.txt",
			},
			ConfigFiles: map[string]string{
				"../testdata/whatever.conf": "/etc/fake/fake.conf",
			},
			EmptyFolders: []string{
				"/var/log/whatever",
				"/usr/share/whatever",
			},
		},
	})
}

func TestArchToAlpine(t *testing.T) {
	verifyArch(t, "", "")
	verifyArch(t, "abc", "abc")
	verifyArch(t, "386", "x86")
	verifyArch(t, "amd64", "x86_64")
	verifyArch(t, "arm", "armhf")
	verifyArch(t, "arm6", "armhf")
	verifyArch(t, "arm7", "armhf")
	verifyArch(t, "arm64", "aarch64")
}

func verifyArch(t *testing.T, nfpmArch, expectedArch string) {
	info := exampleInfo(t)
	info.Arch = nfpmArch

	assert.NoError(t, Default.Package(info, ioutil.Discard))
	assert.Equal(t, expectedArch, info.Arch)
}

func TestCreateBuilderData(t *testing.T) {
	info := exampleInfo(t)
	size := int64(0)
	builderData := createBuilderData(info, &size)

	// tw := tar.NewWriter(ioutil.Discard)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	assert.NoError(t, builderData(tw))

	assert.Equal(t, 1043464, buf.Len())
}

func TestFileToBase64(t *testing.T) {
	base64Key, err := fileToBase64String(getPrivateKeyFile())
	assert.NoError(t, err)
	assert.Equal(t,
		"LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlKS1FJQkFBS0NBZ0VBejI2c3ZmTlVVYm9zbnUzbmh1cjlxcnlTb2tnYmh5cWE4YUFCR1BkT1NpeGt1c2NCCnZsc1hPYVA2UldkWHUrcmFNNUhtUE9JN3YwTmFnMGs1KzZSVWUrWlBGaU5NaHMwbTlCMkpoNS9lak9wSlByMlkKVFNYUHlTenBFVEFJKy9SbWdXY2FkNVY2SUNTSEdlTHRsMjcwZXc4a0dMbjVOUlJ6bk1uUzExcDJRZllDaFdkbApaUUhUbFp2S3J2TDFXUU56TDlBRm50ZnlCWTg1d0VnS0dSZVFLaUluL2wzUWZ1NjJjTjROUnhkd3Bkdm43WmllCmc0MzRicGN0ZG9Ib1RkNlFkd05Udk9sTU1XdWFFeHBVcU1ncFppMU5ZSVFyeE8vR2dXQTR3N1BMbDQ0LzZpNWkKYVZhM2JDYWJEY2RHNEQ5ZFlkRTRPQS9mK0hMUlRHNGp5Nk5WdWY4ejlRbDZMYk5HTW81dDVRWGhyWFZ2TlltOQpCTUJZOEZ6aGdxWlZjSHlhQ0tjV0RWYUVmaEFwTFdyT25oeVFmbW1jRlVrQWZtUGhHWVJoUVZMR0F2amd1eVJCCm9DbUZIaUxsMWUxaThQVlFJL2pQc3Q1eHlOSWV1R2hiTkRjd2djU3hOT0xLTGF2VTVCblk2U0hUUjlqQ2R4L1EKK21SVDdRWUpQYkZqRWlVZXFMVVgrWWNEbmxPQVpTRjVrcmZPWHczK2RjdklLSWJscU5kTDFNR1lvTXVPRGtWOApHT0IzdmlqVlhWTHdmOUpBN1NDU2hRNER2M2p6THB4WTY2VFd6M1VqVDNSemxpSlFUbm1yUWlxbVhXVXJSS3MwCm9HRUZTVGovckk0RVVJT1FnNzkyQ0dRNHNRcXg3U3FxNm40OHJiOFZsQUg5Yis5aUtEUjhhSUEzOFZrQ0F3RUEKQVFLQ0FnQVlBcldZSHl4cGNXVnMyQmp1c3hDOXpLb2tncmc5QXgrQVRJY1QvcnhmTlpoTFRuSFRPUFFOUmYvWQpQTWdaQm14UGY5bm92ajh3T25tbHJMbzdlS0FXMzJmVUppM2JoSyszbmh1blNVZ1hnNThLMWlOaytyVjhrZWhBCmh4RGpLVDBjU1hUMDFxYVdSZVFsaVBEN2tHcFlQRDV2WmtlRWIyT2FpSG9SVjNWTTJVOGRaZ1NFbHB1Sk84bFEKU3VzL2JIak8xZ053aVlxSVBqWHZIZWVkVSs3cUVaNFRnWVI2ek9MdFdhYXJ6ZmpLR2hSVW1rL3U1bVlWVndaNgpLenRhbUNLY3hCUFRVQ1h6cW9MaEp6RVpnR0hhWS9BSzlnR2pBQ1k0SDQweWlnTk0vYmhFUVM0L0J6eWdGaS9vCmZtS2ozbkhPdXNzSklqMUlvdkc3S1J5WG04WjJWY2xrSW5jUHo0T3JXdjRzMjArbXZTdmE0bTdocUh4a0NsS0wKNWZMbzBNcnlpMXlKbzJFck56KzFiZU5rZ09GQ243V1VaN0pZbXJVZlhCcnF3cldhOEJBeHhhWVZGdENGc2tZdApDeDlDaSs4dGQyUTlZWGR4RUZTWGhhTzMxQ0VQK2RHbWc1OGg4YS9EZVQ2OHowdS9xT0hFcFJtVlpvVjg5VjlGCitPdGFDZDg2cXdiVVlkdmh6b2hKWEttbmg1bEx6R29lZ3Z2aFRMWjlqM2tCWGdIclRBSTRKTzdMd0c4SkMweGkKWDRtY3lvSC9qVGV2dEpSdmNjQUR5MFpmZlFvSno2ZVl4eS9EVm9Xbmw0YnJ3ZFJjNHN5Qm9lbE9hbWhLckltUApnMzZpSTc1ZGV3bElUcE1iSkRGd2NDQmpBQkdpQU9PelBEM2UrR1Q2S1Q5ekl2aWFJUUtDQVFFQTY3SmtFM0RJCm4zZFFncEpLVWlJKzArbGoxdHVrdSszN2FVdnZVekVRb1JaQkttYnl3Yk45bkN4SGU4TUFGRExadHg1UUpBMHAKYTZ0K253eE1ZVTdCYi9KTnIvRTg1N2xHeU5NQWl4K2swMEtjOFJZWDgyRmQ2L0VhQnlWaWR0cTBzNG0wREsyNQo2OGpSUWNEaVlkMG1ydHBCVFRuNURoOXR2Mnp6L01vcnRUMUhTZWI0dHhUV095OW9vZmFLUHlPSXlQK2JCZmR2CmtZOHQ1MVVucjJMM2QvSWx5Wm41UjNTQVgrRXhKdUl4bGVDR1hNTmR5OWZQMTJnMXp5elFweXRzVStqM09vVWoKRHJqNmIvbzNRQTcxN0ZKaWtPS3hlU095ZnQ3QkVvRmt6SFc1eGN6WFVhcEdUNjJVOFVoTHFyUmNrUGR2N0hOOApiY2pZNnlRKy9DM0sxUUtDQVFFQTRVejk5M3N0cFhtY1BJa3pZMWpHSkNzc3ExUStvVE9wbXdPVXl2dGhhM3diCmdVM3lzaVlGeFZwYVRnd2szNzQrQWc3eGNhb1hsVVkwdk9UMUJmaW15UUxpSjVSbFFHUmJpeFBlYzVrb3kwUTMKZ1NlR1Y4Um5JSkl3RnJsUUs1NWFLaU9sR3NOZ1dpU2FFTWZYMTF6U1BrOG5GSFIvNGRGTTVhN2UyNkdCdzVpYQo2Zld5Ymc4d2ZnWlh5S1BDdFlTV3hNTVhuU3ZsUi9TOTB1d3ZzL0pDZUZkbDJMaTZMcDRXVzZLRWRJc245cUZDCmpEaUx2d2RKWTk5cnNyV2hRUG5JL0psdFpJQkluU3pqYTRucnh1STZ3SzRVQ0ltcjArRWx0MXA5VEhiSXp3d3IKVWVwcGJ3cUxzTTVVUWJuOFNkTnZJUm9GQ2hoak1xSlZ3aG9mbElSR2RRS0NBUUVBbWRtaFA1dGdLYzk5U3kzbwp1NEpGRnBpREppM0xjeXlkN3BhMWlzMDlPSmxKUWo5ZStKZU1SNVFUdVRLSmE2WGh2WWxZOEo5eXlTaHhoNnBFCmRVUXVPaitrL0ZMdzJhVjBFZ1RCbHc2NXpYanU3dVBvRUdNZkpyTUR0V1J1eUh4c2RjRk9PUFJ4cHZvM3RiOE4KUnFwUDVOVHN5VmN0UGsyL21yT284L3FYMno4N3VINi9IT3JLQ0dvaTE0NFJvYk0xUjFhcHY1UkxURzEwbmt0VQprMFI3bXQwQ1UzMWhYWVlyZ2VxQjVnckNLVDRkRnBJa09Mb1BubUVVdHI1ZkdLL2NqMDFEaS94NTdOTk1EaW43ClJLSS9YdHBNSXAwSEViYitmWmd6MlR1REszOHhHMjlob1pvUE9WVnFJckY1U3QxZWl2WXBKZVFnZFowa0V5RmUKeDhld1hRS0NBUUJMR2ZVV2gvTUJVL1ptbjMySHdsSGFRS0lWUW5IV0huaU0rYmFocXdZZ1pEQnUrK0xJeTYvawp4MmVPMkxGNSs5cURxU09HdGlKQ1dqSytQTHdJajRoWlBTTFIrcjk5cFhaMmQ5c1JRWjY5a3pIRlZiMk1pQ1d3ClQ4ckQ2R1gzQkVRZUE5L0hlaFVtTjBrOENzSENRbWk2Nkh1b2IrVXBDekhNNW12WFhwRDQrR2U3VVhGM0NvMHAKbFVleDFCVFZtU3NBejkrUlBzNmhHODRpL3lRdm9iUFNsWitYakl4VGVkTU9ITEIyZ09TRGErSFpDQWhkVnpwNQpsa0k2UWgxTW9YY0Q3TWp3VldyZktkVnRSWDVZdjVUQ0ljVC95NVNCZm0ycUh2bmhnVDhTOVlXRE90YUdjMGQ1CldtM3ZzdVdNWG5TTzNqT0wxL0ZKTVovUW9oQ2cyeTc1QW9JQkFRRE1ZNFovUkJUa3ViWUVUMUFpYnFzZnUxcnQKdEJld0NTaFFnNHlSRkRDQksrenV2ZnFWTUp4ZGIzS0QxdEtvTzVVN2xIUjlsUzJybTkyUHJhQnZ3TVBHYW5Ncgo5eUlIY1VwNFcyeXNrekNtdGIvYW54RHgwVlJQRFR4S004VXJWS0cxcFNFSE10V2xCd3d6b2kyV003b0hPUWI5CmVkbi94YW4zNHppWDl3WlV0Z0JXUzVMZEhkY0FLYWMyMys1TWR6TGJBaFMxT0k4a2tWTEZ5VHBJeDd1S21BODQKQTdRalRtSVYwQXF2RlVnOHBzdkFpWTR4V3BXeDJKak56blZOaGlZd1dXVTltK0lhM3QwUWo1SFphM2RBcDRpZApwL2trKzRSdzV2U2tMTkk5VjhpOWtSZnJFcWlrb2VBYVBQK25uaGk1TEI4bWVjOTN1Mko3RzRXZjNxNDYKLS0tLS1FTkQgUlNBIFBSSVZBVEUgS0VZLS0tLS0KCg==", //nolint:lll
		base64Key)
}

func TestCreateSignature(t *testing.T) {
	var signatureTgz bytes.Buffer
	digest := sha256.New()
	controlDigest := digest.Sum(nil)
	info := &nfpm.Info{
		PrivateKeyBase64: getBase64PrivateKey(t),
	}
	assert.NoError(t, createSignature(&signatureTgz, controlDigest, info))

	assert.Equal(t, 666, signatureTgz.Len())
	assert.Equal(t, 32, digest.Size())
}

func TestCreateSignatureFromKeyFile(t *testing.T) {
	var signatureTgz bytes.Buffer
	digest := sha256.New()
	controlDigest := digest.Sum(nil)
	info := &nfpm.Info{
		PrivateKeyFile: getPrivateKeyFile(),
	}
	assert.NoError(t, createSignature(&signatureTgz, controlDigest, info))

	assert.Equal(t, 666, signatureTgz.Len())
	assert.Equal(t, 32, digest.Size())
}

func TestCreateSignatureKeyPrecedence(t *testing.T) {
	var signatureTgz bytes.Buffer
	digest := sha256.New()
	controlDigest := digest.Sum(nil)
	info := &nfpm.Info{
		PrivateKeyBase64: getBase64PrivateKey(t),
		PrivateKeyFile:   "bogus key file",
	}
	assert.NoError(t, createSignature(&signatureTgz, controlDigest, info))

	assert.Equal(t, 666, signatureTgz.Len())
	assert.Equal(t, 32, digest.Size())
}

func TestCombineToApk(t *testing.T) {
	var bufData bytes.Buffer
	bufData.Write([]byte{1})

	var bufControl bytes.Buffer
	bufControl.Write([]byte{2})

	var bufSignature bytes.Buffer
	bufSignature.Write([]byte{3})

	var bufTarget bytes.Buffer

	assert.NoError(t, combineToApk(&bufTarget, &bufData, &bufControl, &bufSignature))

	assert.Equal(t, 3, bufTarget.Len())
}

func TestPathsToCreate(t *testing.T) {
	for pathToTest, parts := range map[string][]string{
		"/usr/share/doc/whatever/foo.md": {"usr", "usr/share", "usr/share/doc", "usr/share/doc/whatever"},
		"/var/moises":                    {"var"},
		"/":                              []string(nil),
	} {
		parts := parts
		pathToTest := pathToTest
		t.Run(fmt.Sprintf("pathToTest: '%s'", pathToTest), func(t *testing.T) {
			assert.Equal(t, parts, pathsToCreate(pathToTest))
		})
	}
}

func TestDefaultWithArch(t *testing.T) {
	for _, arch := range []string{"386", "amd64"} {
		arch := arch
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo(t)
			info.Arch = arch
			var err = Default.Package(info, ioutil.Discard)
			assert.NoError(t, err)
		})
	}
}

func TestNoInfo(t *testing.T) {
	var err = Default.Package(nfpm.WithDefaults(&nfpm.Info{}), ioutil.Discard)
	assert.EqualError(t, err, "failed to parse PEM block containing the key")
}

func TestFileDoesNotExist(t *testing.T) {
	var err = Default.Package(
		nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Arch:        "amd64",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			Overridables: nfpm.Overridables{
				Depends: []string{
					"bash",
				},
				Files: map[string]string{
					"../testdata/": "/usr/local/bin/fake",
				},
				ConfigFiles: map[string]string{
					"../testdata/whatever.confzzz": "/etc/fake/fake.conf",
				},
			},
		}),
		ioutil.Discard,
	)
	assert.EqualError(t, err, "../testdata/whatever.confzzz: file does not exist")
}

func TestNoFiles(t *testing.T) {
	var err = Default.Package(
		nfpm.WithDefaults(&nfpm.Info{
			Name:             "foo",
			Arch:             "amd64",
			Description:      "Foo does things",
			Priority:         "extra",
			Maintainer:       "Carlos A Becker <pkg@carlosbecker.com>",
			Version:          "1.0.0",
			Section:          "default",
			Homepage:         "http://carlosbecker.com",
			Vendor:           "nope",
			PrivateKeyBase64: getBase64PrivateKey(t),
			Overridables: nfpm.Overridables{
				Depends: []string{
					"bash",
				},
			},
		}),
		ioutil.Discard,
	)
	assert.NoError(t, err)
}

func TestCreateBuilderControl(t *testing.T) {
	info := exampleInfo(t)
	size := int64(12345)
	digest := sha256.New()
	dataDigest := digest.Sum(nil)
	builderControl := createBuilderControl(info, size, dataDigest)

	var controlTgz bytes.Buffer
	tw := tar.NewWriter(&controlTgz)
	assert.NoError(t, builderControl(tw))

	assert.Equal(t, 798, controlTgz.Len())

	stringControlTgz := controlTgz.String()
	assert.True(t, strings.HasPrefix(stringControlTgz, ".PKGINFO"))
	assert.Contains(t, stringControlTgz, "pkgname = "+info.Name)
	assert.Contains(t, stringControlTgz, "pkgver = "+info.Version+"-"+info.Release)
	assert.Contains(t, stringControlTgz, "pkgdesc = "+info.Description)
	assert.Contains(t, stringControlTgz, "url = "+info.Homepage)
	assert.Contains(t, stringControlTgz, "maintainer = "+info.Maintainer)
	assert.Contains(t, stringControlTgz, "replaces = "+info.Replaces[0])
	assert.Contains(t, stringControlTgz, "provides = "+info.Provides[0])
	assert.Contains(t, stringControlTgz, "depend = "+info.Depends[0])
	assert.Contains(t, stringControlTgz, "arch = "+info.Arch) // conversion using archToAlpine[info.Arch]) would occur in Package() method
	assert.Contains(t, stringControlTgz, "size = "+strconv.Itoa(int(size)))
	assert.Contains(t, stringControlTgz, "datahash = "+hex.EncodeToString(dataDigest))
}

func TestCreateBuilderControlScripts(t *testing.T) {
	info := exampleInfo(t)
	info.Scripts = nfpm.Scripts{
		PreInstall:  "../testdata/scripts/preinstall.sh",
		PostInstall: "../testdata/scripts/postinstall.sh",
		PreRemove:   "../testdata/scripts/preremove.sh",
		PostRemove:  "../testdata/scripts/postremove.sh",
	}

	size := int64(12345)
	digest := sha256.New()
	dataDigest := digest.Sum(nil)
	builderControl := createBuilderControl(info, size, dataDigest)

	var controlTgz bytes.Buffer
	tw := tar.NewWriter(&controlTgz)
	assert.NoError(t, builderControl(tw))

	stringControlTgz := controlTgz.String()
	assert.Contains(t, stringControlTgz, ".pre-install")
	assert.Contains(t, stringControlTgz, ".post-install")
	assert.Contains(t, stringControlTgz, ".pre-deinstall")
	assert.Contains(t, stringControlTgz, ".post-deinstall")
	assert.Contains(t, stringControlTgz, "datahash = "+hex.EncodeToString(dataDigest))
}

func TestControl(t *testing.T) {
	digest := sha256.New()
	dataDigest := digest.Sum(nil)

	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
		Info:          exampleInfo(t),
		InstalledSize: 10,
		Datahash:      hex.EncodeToString(dataDigest),
	}))
	var golden = "testdata/control.golden"
	if *updateApk {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0655))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	assert.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}
