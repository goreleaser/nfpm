package signatures

import (
	"bytes"
	"io"
	"io/ioutil"
	"unicode"

	"github.com/pkg/errors"
	"golang.org/x/crypto/openpgp"
)

// PGPSigner returns a PGP signer that creates a detached non-ASCII-armored
// signature and is compatible with rpmpack's signature API.
func PGPSigner(keyFile, passphrase string) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		key, err := readSigningKey(keyFile, passphrase)
		if err != nil {
			return nil, errors.Wrap(err, "detach sign")
		}

		var signature bytes.Buffer

		err = openpgp.DetachSign(&signature, key, bytes.NewReader(data), nil)
		if err != nil {
			return nil, errors.Wrap(err, "detach sign")
		}

		return signature.Bytes(), nil
	}
}

// ArmoredDetachSign creates an ASCII-armored detached signature.
func ArmoredDetachSign(message io.Reader, keyFile, passphrase string) ([]byte, error) {
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
