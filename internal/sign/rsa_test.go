package sign

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
	require.EqualError(t, err, "key is encrypted but no passphrase was provided")
}

func TestInvalidHash(t *testing.T) {
	invalidDigest := []byte("test")
	_, err := RSASignSHA1Digest(invalidDigest, "testdata/rsa.priv", "hunter2")
	require.EqualError(t, err, "digest is not a SHA1 hash")
}

func TestRSAVerifySHA1DigestError(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 12)
	if err != nil {
		t.Fatal(err)
	}
	asn1Bytes, err := asn1.Marshal(rsaKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	x509Bytes, err := x509.MarshalPKIXPublicKey(&ecdsaKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	pemKeys := []*pem.Block{
		{
			Type:  "PUBLIC KEY",
			Bytes: asn1Bytes,
		},
		{
			Type:  "PUBLIC KEY",
			Bytes: x509Bytes,
		},
	}

	for _, pemKey := range pemKeys {
		cwd, _ := os.Getwd()

		keyFile, err := ioutil.TempFile(cwd, "*.pem")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(keyFile.Name())
		pem.Encode(keyFile, pemKey)
		keyFile.Close()
		digest := sha1.New().Sum(nil)
		assert.Error(t, RSAVerifySHA1Digest(digest, []byte{}, keyFile.Name()))
	}
}
