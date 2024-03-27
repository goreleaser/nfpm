package sign

import (
	"bytes"
	"crypto/sha1" // nolint:gosec
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
		{"unprotected pkcs1", "testdata/rsa_unprotected.priv", "testdata/rsa_unprotected.pub", ""},
		{"protected pkcs1", "testdata/rsa.priv", "testdata/rsa.pub", pass},
		{"unprotected pkcs8", "testdata/rsa_pkcs8.priv", "testdata/rsa_pkcs8.pub", ""},
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
	require.EqualError(t, err, "key is encrypted but no passphrase was provided")
}

func TestInvalidHash(t *testing.T) {
	invalidDigest := []byte("test")
	_, err := RSASignSHA1Digest(invalidDigest, "testdata/rsa.priv", "hunter2")
	require.EqualError(t, err, "digest is not a SHA1 hash")
}

func TestRSAVerifyWrongKey(t *testing.T) {
	digest := sha1.New().Sum(nil) // nolint:gosec

	testCases := []struct {
		name    string
		privKey string
		pubKey  string
	}{
		{"pkcs1", "testdata/rsa_unprotected.priv", "testdata/rsa_unprotected.pub"},
		{"pkcs8", "testdata/rsa_pkcs8.priv", "testdata/rsa_pkcs8.pub"},
	}

	for _, testCase := range testCases {
		sig, err := rsaSign(bytes.NewReader(digest), testCase.privKey, "")
		require.NoError(t, err)

		err = RSAVerifySHA1Digest(digest, sig, testCase.pubKey)
		require.EqualError(t, err, "verify PKCS1v15 signature: crypto/rsa: verification error")
	}
}

func TestRSAVerifyWrongSignature(t *testing.T) {
	digest := sha1.New().Sum(nil) // nolint:gosec

	testCases := []struct {
		name   string
		pubKey string
	}{
		{"pkcs1", "testdata/rsa.pub"},
		{"pkcs8", "testdata/rsa_pkcs8.pub"},
	}

	for _, testCase := range testCases {
		err := RSAVerifySHA1Digest(digest, []byte{}, testCase.pubKey)
		require.EqualError(t, err, "verify PKCS1v15 signature: crypto/rsa: verification error")
	}
}

func TestRSAVerifyWrongPublicKeyFormat(t *testing.T) {
	digest := sha1.New().Sum(nil) // nolint:gosec

	sig, err := rsaSign(bytes.NewReader(digest), "testdata/rsa_unprotected.priv", "")
	require.NoError(t, err)

	err = RSAVerifySHA1Digest(digest, sig, "testdata/wrong_key_format.pub")
	require.Equal(t, err, errNoRSAKey)
}

func TestRSAVerifyWrongSecretKeyFormat(t *testing.T) {
	digest := sha1.New().Sum(nil) // nolint:gosec

	_, err := rsaSign(bytes.NewReader(digest), "testdata/wrong_key_format.priv", "")
	require.Error(t, err)
}
