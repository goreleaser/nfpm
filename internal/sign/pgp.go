package sign

import (
	"bytes"
	"io"
	"io/ioutil"
	"unicode"

	"github.com/pkg/errors"
	"golang.org/x/crypto/openpgp"

	"github.com/goreleaser/nfpm"
)

// PGPSigner returns a PGP signer that creates a detached non-ASCII-armored
// signature and is compatible with rpmpack's signature API.
func PGPSigner(keyFile, passphrase string) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		key, err := readSigningKey(keyFile, passphrase)
		if err != nil {
			return nil, &nfpm.ErrSigningFailure{Err: err}
		}

		var signature bytes.Buffer

		err = openpgp.DetachSign(&signature, key, bytes.NewReader(data), nil)
		if err != nil {
			return nil, &nfpm.ErrSigningFailure{Err: err}
		}

		return signature.Bytes(), nil
	}
}

// PGPArmoredDetachSign creates an ASCII-armored detached signature.
func PGPArmoredDetachSign(message io.Reader, keyFile, passphrase string) ([]byte, error) {
	key, err := readSigningKey(keyFile, passphrase)
	if err != nil {
		return nil, errors.Wrap(err, "armored detach sign")
	}

	var signature bytes.Buffer

	err = openpgp.ArmoredDetachSign(&signature, key, message, nil)
	if err != nil {
		return nil, errors.Wrap(err, "armored detach sign")
	}

	return signature.Bytes(), nil
}

// PGPVerify is exported for use in tests and verifies a ASCII-armored or non-ASCII-armored
// signature using an ASCII-armored or non-ASCII-armored public key file. The signer
// identity is not explicitly checked, other that the obvious fact that the signer's key must
// be in the armoredPubKeyFile.
func PGPVerify(message io.Reader, signature []byte, armoredPubKeyFile string) error {
	keyFileContent, err := ioutil.ReadFile(armoredPubKeyFile)
	if err != nil {
		return errors.Wrap(err, "reading armored public key file")
	}

	var keyring openpgp.EntityList

	if isASCII(keyFileContent) {
		keyring, err = openpgp.ReadArmoredKeyRing(bytes.NewReader(keyFileContent))
		if err != nil {
			return errors.Wrap(err, "decoding armored public key file")
		}
	} else {
		keyring, err = openpgp.ReadKeyRing(bytes.NewReader(keyFileContent))
		if err != nil {
			return errors.Wrap(err, "decoding public key file")
		}
	}

	if isASCII(signature) {
		_, err = openpgp.CheckArmoredDetachedSignature(keyring, message, bytes.NewReader(signature))
		return err
	}

	_, err = openpgp.CheckDetachedSignature(keyring, message, bytes.NewReader(signature))
	return err
}

func readSigningKey(keyFile, passphrase string) (*openpgp.Entity, error) {
	fileContent, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, errors.Wrap(err, "reading PGP key file")
	}

	var entityList openpgp.EntityList

	if isASCII(fileContent) {
		entityList, err = openpgp.ReadArmoredKeyRing(bytes.NewReader(fileContent))
		if err != nil {
			return nil, errors.Wrap(err, "decoding armored PGP keyring")
		}
	} else {
		entityList, err = openpgp.ReadKeyRing(bytes.NewReader(fileContent))
		if err != nil {
			return nil, errors.Wrap(err, "decoding PGP keyring")
		}
	}

	var key *openpgp.Entity

	for _, candidate := range entityList {
		if candidate.PrivateKey == nil {
			continue
		}

		if !candidate.PrivateKey.CanSign() {
			continue
		}

		if key != nil {
			return nil, errors.New("more than one signing key in keyring")
		}

		key = candidate
	}

	if key == nil {
		return nil, errors.New("no signing key in keyring")
	}

	if key.PrivateKey.Encrypted {
		if passphrase == "" {
			return nil, errors.New("key is encrypted but no passphrase was provided")
		}

		err = key.PrivateKey.Decrypt([]byte(passphrase))
		if err != nil {
			return nil, errors.Wrap(err, "decrypt secret signing key")
		}
	}

	return key, nil
}

func isASCII(s []byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}
