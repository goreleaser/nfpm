package sign

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRSASignAndVerify(t *testing.T) {
	testData := []byte("test")

	testCases := []struct {
		name       string
		privKey    string
		pubKey     string
		passphrase string
	}{
		{"unprotected", "testdata/rsa_unprotected.priv", "testdata/rsa_unprotected.pub", ""},
		{"protected", "testdata/rsa.priv", "testdata/rsa.pub", pass},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			sig, err := rsaSign(bytes.NewReader(testData), testCase.privKey, testCase.passphrase)
			require.NoError(t, err)

			err = rsaVerify(bytes.NewReader(testData), sig, testCase.pubKey)
			require.NoError(t, err)
		})
	}
}

func TestWrongPassphrase(t *testing.T) {
	testData := []byte("test")
	_, err := rsaSign(bytes.NewReader(testData), "testdata/rsa.priv", "password123")
	require.EqualError(t, err, "decrypt private key PEM block: x509: decryption password incorrect")
}

func TestNoPassphrase(t *testing.T) {
	testData := []byte("test")
	_, err := rsaSign(bytes.NewReader(testData), "testdata/rsa.priv", "")
	require.EqualError(t, err, "key is encrypted no passphrase provided")
}

func TestInvalidHash(t *testing.T) {
	invalidDigest := []byte("test")
	_, err := RSASignSHA1Digest(invalidDigest, "testdata/rsa.priv", "hunter2")
	require.EqualError(t, err, "digest is not a SHA256 hash")
}
