package sign

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // nolint:gosec
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	errNoPemBlock      = errors.New("no PEM block found")
	errDigestNotSH1    = errors.New("digest is not a SHA1 hash")
	errDigestNotSHA256 = errors.New("digest is not a SHA256 hash")
	errNoPassphrase    = errors.New("key is encrypted but no passphrase was provided")
	errNoRSAKey        = errors.New("key is not an RSA key")
)

const (
	PKCS1PrivkeyPreamble = "RSA PRIVATE KEY"
	PKCS8PrivkeyPreamble = "PRIVATE KEY"
)

// RSASignSHA1Digest signs the provided SHA1 message digest. The key file
// must be in the PEM format and can either be encrypted or not.
func RSASignSHA1Digest(sha1Digest []byte, keyFile, passphrase string) ([]byte, error) {
	return rsaSignDigest(sha1Digest, keyFile, passphrase, crypto.SHA1, sha1.Size, errDigestNotSH1)
}

// RSASignSHA256Digest signs the provided SHA256 message digest. The key file
// must be in the PEM format and can either be encrypted or not.
func RSASignSHA256Digest(sha256Digest []byte, keyFile, passphrase string) ([]byte, error) {
	return rsaSignDigest(sha256Digest, keyFile, passphrase, crypto.SHA256, sha256.Size, errDigestNotSHA256)
}

func rsaSignDigest(digest []byte, keyFile, passphrase string, hash crypto.Hash, digestSize int, digestErr error) ([]byte, error) {
	if len(digest) != digestSize {
		return nil, digestErr
	}

	priv, err := loadPrivateKey(keyFile, passphrase)
	if err != nil {
		return nil, err
	}

	signature, err := priv.Sign(rand.Reader, digest, hash)
	if err != nil {
		return nil, fmt.Errorf("signing: %w", err)
	}

	return signature, nil
}

func loadPrivateKey(keyFile, passphrase string) (crypto.Signer, error) {
	keyFileContent, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("reading key file: %w", err)
	}
	block, _ := pem.Decode(keyFileContent)
	if block == nil {
		return nil, errNoPemBlock
	}

	blockData := block.Bytes
	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck
		if passphrase == "" {
			return nil, errNoPassphrase
		}
		decryptedBlockData, err := x509.DecryptPEMBlock(block, []byte(passphrase)) //nolint:staticcheck
		if err != nil {
			return nil, fmt.Errorf("decrypt private key PEM block: %w", err)
		}
		blockData = decryptedBlockData
	}

	switch block.Type {
	case PKCS1PrivkeyPreamble:
		return x509.ParsePKCS1PrivateKey(blockData)
	case PKCS8PrivkeyPreamble:
		privAny, err := x509.ParsePKCS8PrivateKey(blockData)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
		}
		priv, ok := privAny.(*rsa.PrivateKey)
		if !ok {
			return nil, errNoRSAKey
		}
		return priv, nil
	default:
		return nil, fmt.Errorf(`key type "%v" is not supported`, block.Type)
	}
}

func rsaSign(message io.Reader, keyFile, passphrase string) ([]byte, error) {
	sha1Hash := sha1.New() // nolint:gosec
	_, err := io.Copy(sha1Hash, message)
	if err != nil {
		return nil, fmt.Errorf("create SHA1 message digest: %w", err)
	}

	return RSASignSHA1Digest(sha1Hash.Sum(nil), keyFile, passphrase)
}

// RSAVerifySHA1Digest is exported for use in tests and verifies a signature over the
// provided SHA1 hash of a message. The key file must be in the PEM format.
func RSAVerifySHA1Digest(sha1Digest, signature []byte, publicKeyFile string) error {
	return rsaVerifyDigest(sha1Digest, signature, publicKeyFile, crypto.SHA1, sha1.Size, errDigestNotSH1)
}

// RSAVerifySHA256Digest is exported for use in tests and verifies a signature over the
// provided SHA256 hash of a message. The key file must be in the PEM format.
func RSAVerifySHA256Digest(sha256Digest, signature []byte, publicKeyFile string) error {
	return rsaVerifyDigest(sha256Digest, signature, publicKeyFile, crypto.SHA256, sha256.Size, errDigestNotSHA256)
}

func rsaVerifyDigest(digest, signature []byte, publicKeyFile string, hash crypto.Hash, digestSize int, digestErr error) error {
	if len(digest) != digestSize {
		return digestErr
	}

	rsaPub, err := loadRSAPublicKey(publicKeyFile)
	if err != nil {
		return err
	}

	err = rsa.VerifyPKCS1v15(rsaPub, hash, digest, signature)
	if err != nil {
		return fmt.Errorf("verify PKCS1v15 signature: %w", err)
	}

	return nil
}

func loadRSAPublicKey(publicKeyFile string) (*rsa.PublicKey, error) {
	keyFileContent, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return nil, fmt.Errorf("reading key file: %w", err)
	}
	block, _ := pem.Decode(keyFileContent)
	if block == nil {
		return nil, errNoPemBlock
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKIX public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errNoRSAKey
	}

	return rsaPub, nil
}

func rsaVerify(message io.Reader, signature []byte, publicKeyFile string) error {
	sha1Hash := sha1.New() // nolint:gosec
	_, err := io.Copy(sha1Hash, message)
	if err != nil {
		return fmt.Errorf("create SHA1 message digest: %w", err)
	}

	return RSAVerifySHA1Digest(sha1Hash.Sum(nil), signature, publicKeyFile)
}
