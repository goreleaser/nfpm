package sign

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1" // nolint:gosec
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
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

func TestRSASignAndVerifySHA256Digest(t *testing.T) {
	testData := []byte("test")
	digest := sha256.Sum256(testData)
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
		t.Run(testCase.name, func(t *testing.T) {
			sig, err := RSASignSHA256Digest(digest[:], testCase.privKey, testCase.passphrase)
			require.NoError(t, err)
			require.NoError(t, RSAVerifySHA256Digest(digest[:], sig, testCase.pubKey))
		})
	}
}

func TestInvalidSHA256Hash(t *testing.T) {
	invalidDigest := []byte("test")
	_, err := RSASignSHA256Digest(invalidDigest, "testdata/rsa.priv", "hunter2")
	require.EqualError(t, err, "digest is not a SHA256 hash")

	err = RSAVerifySHA256Digest(invalidDigest, []byte{}, "testdata/rsa.pub")
	require.EqualError(t, err, "digest is not a SHA256 hash")
}

func TestRSAVerifySHA256WrongSignature(t *testing.T) {
	digest := sha256.Sum256([]byte("test"))
	err := RSAVerifySHA256Digest(digest[:], []byte{}, "testdata/rsa.pub")
	require.EqualError(t, err, "verify PKCS1v15 signature: crypto/rsa: verification error")
}

func TestLoadPrivateKeyFileNotFound(t *testing.T) {
	_, err := loadPrivateKey("testdata/does_not_exist.priv", "")
	require.ErrorContains(t, err, "reading key file")
}

func TestLoadPrivateKeyNoPEMBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.priv")
	require.NoError(t, os.WriteFile(path, []byte("not a pem block"), 0o600))
	_, err := loadPrivateKey(path, "")
	require.ErrorIs(t, err, errNoPemBlock)
}

func TestLoadPrivateKeyPKCS8ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_pkcs8.priv")
	bad := pem.EncodeToMemory(&pem.Block{Type: PKCS8PrivkeyPreamble, Bytes: []byte("not-valid-der")})
	require.NoError(t, os.WriteFile(path, bad, 0o600))
	_, err := loadPrivateKey(path, "")
	require.ErrorContains(t, err, "parse PKCS#8 private key")
}

func TestLoadPrivateKeyRejectsNonRSAPKCS8(t *testing.T) {
	// A PKCS#8 ECDSA key parses fine but is not RSA; since every signing and
	// verification path is RSA-only, loadPrivateKey must reject it up front
	// rather than emit a signature no RSA verifier can validate.
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)
	dir := t.TempDir()
	path := filepath.Join(dir, "ecdsa_pkcs8.priv")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: PKCS8PrivkeyPreamble, Bytes: der})
	require.NoError(t, os.WriteFile(path, pemBytes, 0o600))

	_, err = loadPrivateKey(path, "")
	require.ErrorIs(t, err, errNoRSAKey)
}

func TestLoadRSAPublicKeyFileNotFound(t *testing.T) {
	_, err := loadRSAPublicKey("testdata/does_not_exist.pub")
	require.ErrorContains(t, err, "reading key file")
}

func TestLoadRSAPublicKeyNoPEMBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.pub")
	require.NoError(t, os.WriteFile(path, []byte("not a pem block"), 0o600))
	_, err := loadRSAPublicKey(path)
	require.ErrorIs(t, err, errNoPemBlock)
}

func TestLoadRSAPublicKeyParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pub")
	bad := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("not-valid-der")})
	require.NoError(t, os.WriteFile(path, bad, 0o600))
	_, err := loadRSAPublicKey(path)
	require.ErrorContains(t, err, "parse PKIX public key")
}
