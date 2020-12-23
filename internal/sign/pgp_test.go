package sign

import (
	"bytes"
	"errors"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/openpgp"

	"github.com/goreleaser/nfpm/v2"
)

const pass = "hunter2"

func TestPGPSignerAndVerify(t *testing.T) {
	data := []byte("testdata")
	verifierKeyring := readArmoredKeyring(t, "testdata/pubkey.asc")

	testCases := []struct {
		name    string
		keyFile string
		pass    string
	}{
		{"protected", "testdata/privkey.gpg", pass},
		{"unprotected", "testdata/privkey_unprotected.gpg", ""},
		{"armored protected", "testdata/privkey.asc", pass},
		{"armored unprotected", "testdata/privkey_unprotected.asc", ""},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			sig, err := PGPSigner(testCase.keyFile, testCase.pass)(data)
			require.NoError(t, err)

			_, err = openpgp.CheckDetachedSignature(verifierKeyring,
				bytes.NewReader(data), bytes.NewReader(sig))
			assert.NoError(t, err)

			err = PGPVerify(bytes.NewReader(data), sig, "testdata/pubkey.asc")
			assert.NoError(t, err)

			err = PGPVerify(bytes.NewReader(data), sig, "testdata/pubkey.gpg")
			assert.NoError(t, err)
		})
	}
}

func TestArmoredDetachSignAndVerify(t *testing.T) {
	data := []byte("testdata")
	verifierKeyring := readArmoredKeyring(t, "testdata/pubkey.asc")

	testCases := []struct {
		name    string
		keyFile string
		pass    string
	}{
		{"protected", "testdata/privkey.gpg", pass},
		{"unprotected", "testdata/privkey_unprotected.gpg", ""},
		{"armored protected", "testdata/privkey.asc", pass},
		{"armored unprotected", "testdata/privkey_unprotected.asc", ""},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			sig, err := PGPArmoredDetachSign(bytes.NewReader(data),
				testCase.keyFile, testCase.pass)
			require.NoError(t, err)

			_, err = openpgp.CheckArmoredDetachedSignature(verifierKeyring,
				bytes.NewReader(data), bytes.NewReader(sig))
			assert.NoError(t, err)

			err = PGPVerify(bytes.NewReader(data), sig, "testdata/pubkey.asc")
			assert.NoError(t, err)

			err = PGPVerify(bytes.NewReader(data), sig, "testdata/pubkey.gpg")
			assert.NoError(t, err)
		})
	}
}

func readArmoredKeyring(t *testing.T, fileName string) openpgp.EntityList {
	t.Helper()
	content, err := ioutil.ReadFile(fileName)
	require.NoError(t, err)

	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(content))
	require.NoError(t, err)

	return keyring
}

func TestPGPSignerError(t *testing.T) {
	_, err := PGPSigner("/does/not/exist", "")([]byte("data"))
	require.Error(t, err)

	var expectedError *nfpm.ErrSigningFailure
	assert.True(t, errors.As(err, &expectedError))
}

func TestNoSigningKey(t *testing.T) {
	_, err := readSigningKey("testdata/pubkey.asc", pass)
	require.EqualError(t, err, "no signing key in keyring")
}

func TestMultipleKeys(t *testing.T) {
	_, err := readSigningKey("testdata/multiple_privkeys.asc", pass)
	require.EqualError(t, err, "more than one signing key in keyring")
}

func TestWrongPass(t *testing.T) {
	_, err := readSigningKey("testdata/privkey.asc", "password123")
	require.EqualError(t, err,
		"decrypt secret signing key: openpgp: invalid data: private key checksum failure")
}

func TestEmptyPass(t *testing.T) {
	_, err := readSigningKey("testdata/privkey.asc", "")
	require.EqualError(t, err, "key is encrypted but no passphrase was provided")
}

func TestReadArmoredKey(t *testing.T) {
	_, err := readSigningKey("testdata/privkey.asc", pass)
	require.NoError(t, err)
}

func TestReadKey(t *testing.T) {
	_, err := readSigningKey("testdata/privkey.gpg", pass)
	require.NoError(t, err)
}

func TestIsASCII(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/privkey.asc")
	require.NoError(t, err)
	assert.True(t, isASCII(data))

	data, err = ioutil.ReadFile("testdata/privkey.gpg")
	require.NoError(t, err)
	assert.False(t, isASCII(data))
}
