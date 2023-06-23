package sign

import (
	"bytes"
	"crypto"
	"fmt"
	"os"

	"github.com/goreleaser/nfpm/v2"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/packet"
)

// PGPSignerWithKeyID returns a PGP signer that creates a detached non-ASCII-armored
// signature and is compatible with rpmpack's signature API.
func PGPSignerWithKeyID(keyFile, passphrase string, hexKeyID *string) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		if _, err := parseKeyID(hexKeyID); err != nil {
			return nil, fmt.Errorf("%v is not a valid key id: %w", hexKeyID, err)
		}

		key, err := readGoSigningKey(keyFile, passphrase)
		if err != nil {
			return nil, &nfpm.ErrSigningFailure{Err: err}
		}

		var signature bytes.Buffer

		err = openpgp.DetachSign(&signature, key, bytes.NewReader(data), &packet.Config{
			DefaultHash: crypto.SHA256,
		})
		if err != nil {
			return nil, &nfpm.ErrSigningFailure{Err: err}
		}

		return signature.Bytes(), nil
	}
}

func readGoSigningKey(keyFile, passphrase string) (*openpgp.Entity, error) {
	fileContent, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("reading PGP key file: %w", err)
	}

	var entityList openpgp.EntityList

	if isASCII(fileContent) {
		entityList, err = openpgp.ReadArmoredKeyRing(bytes.NewReader(fileContent))
		if err != nil {
			return nil, fmt.Errorf("decoding armored PGP keyring: %w", err)
		}
	} else {
		entityList, err = openpgp.ReadKeyRing(bytes.NewReader(fileContent))
		if err != nil {
			return nil, fmt.Errorf("decoding PGP keyring: %w", err)
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
			return nil, errMoreThanOneKey
		}

		key = candidate
	}

	if key == nil {
		return nil, errNoKeys
	}

	if key.PrivateKey.Encrypted {
		if passphrase == "" {
			return nil, errNoPassword
		}
		pw := []byte(passphrase)
		err = key.PrivateKey.Decrypt(pw)
		if err != nil {
			return nil, fmt.Errorf("decrypt secret signing key: %w", err)
		}
		for _, sub := range key.Subkeys {
			if sub.PrivateKey != nil {
				if err := sub.PrivateKey.Decrypt(pw); err != nil {
					return nil, fmt.Errorf("gopenpgp: error in unlocking sub key: %w", err)
				}
			}
		}
	}

	return key, nil
}
