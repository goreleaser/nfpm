package sign

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"io"
	"io/ioutil"

	"github.com/pkg/errors"
)

// RSASign signs the SHA256 hash of the provided message. The key file must be
// in the PEM format and can either be encrypted or not.
func RSASign(message io.Reader, keyFile, passphrase string) ([]byte, error) {
	keyFileContent, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, errors.Wrap(err, "reading key file")
	}

	block, _ := pem.Decode(keyFileContent)
	if block == nil {
		return nil, errors.New("parse PEM block with private key")
	}

	blockData := block.Bytes
	if x509.IsEncryptedPEMBlock(block) {
		if passphrase == "" {
			return nil, errors.New("key is encrypted no passphrase provided")
		}

		var decryptedBlockData []byte

		decryptedBlockData, err = x509.DecryptPEMBlock(block, []byte(passphrase))
		if err != nil {
			return nil, errors.Wrap(err, "decrypt private key PEM block")
		}

		blockData = decryptedBlockData
	}

	priv, err := x509.ParsePKCS1PrivateKey(blockData)
	if err != nil {
		return nil, errors.Wrap(err, "parse PKCS1 private key")
	}

	sha256Hash := sha256.New()
	_, err = io.Copy(sha256Hash, message)
	if err != nil {
		return nil, errors.Wrap(err, "create SHA256 message digest")
	}

	signature, err := priv.Sign(rand.Reader, sha256Hash.Sum(nil), crypto.SHA256)
	if err != nil {
		return nil, errors.Wrap(err, "signing")
	}

	return signature, nil
}

// RSAVerify is exported for use in tests and verifies a signature over the SHA256
// hash of the provided message. The key file must be in the PEM format.
func RSAVerify(message io.Reader, signature []byte, publicKeyFile string) error {
	keyFileContent, err := ioutil.ReadFile(publicKeyFile)
	if err != nil {
		return errors.Wrap(err, "reading key file")
	}

	block, _ := pem.Decode(keyFileContent)
	if block == nil {
		return errors.New("parse PEM block with public key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return errors.Wrap(err, "parse PKIX public key")
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return errors.Wrap(err, "public key is no RSA key")
	}

	sha256Hash := sha256.New()
	_, err = io.Copy(sha256Hash, message)
	if err != nil {
		return errors.Wrap(err, "create SHA256 message digest")
	}

	err = rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, sha256Hash.Sum(nil), signature)
	if err != nil {
		return errors.Wrap(err, "verify PKCS1v15 signature")
	}

	return nil
}
