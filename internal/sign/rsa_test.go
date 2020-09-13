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
			sig, err := RSASign(bytes.NewReader(testData), testCase.privKey, testCase.passphrase)
			require.NoError(t, err)

			err = RSAVerify(bytes.NewReader(testData), sig, testCase.pubKey)
			require.NoError(t, err)
		})
	}
}

func TestWrongPassphrase(t *testing.T) {
	testData := []byte("test")
	_, err := RSASign(bytes.NewReader(testData), "testdata/rsa.priv", "password123")
	require.Error(t, err, "x509: decryption password incorrect")
}

func TestNoPassphrase(t *testing.T) {
	testData := []byte("test")
	_, err := RSASign(bytes.NewReader(testData), "testdata/rsa.priv", "password123")
	require.Error(t, err, "key is encrypted no passphrase provided")
}
