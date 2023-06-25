package sign

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/goreleaser/nfpm/v2"
	"github.com/stretchr/testify/require"
)

const pass = "hunter2"

var testCases = []struct {
	name        string
	privKeyFile string
	pubKeyFile  string
	pass        string
	keyID       *string
}{
	{"protected", "testdata/privkey.gpg", "testdata/pubkey", pass, nil},
	{"unprotected", "testdata/privkey_unprotected.gpg", "testdata/pubkey", "", nil},
	{"armored protected", "testdata/privkey.asc", "testdata/pubkey", pass, nil},
	{"armored unprotected", "testdata/privkey_unprotected.asc", "testdata/pubkey", "", nil},
	{"gpg subkey unprotected", "testdata/privkey_unprotected_subkey_only.asc", "testdata/pubkey", "", nil},
	{"protected-with-key-id", "testdata/privkey.gpg", "testdata/pubkey", pass, pointer.ToString("bc8acdd415bd80b3")},
	{"unprotected-with-key-id", "testdata/privkey_unprotected.gpg", "testdata/pubkey", "", pointer.ToString("bc8acdd415bd80b3")},
	{"armored protected-with-key-id", "testdata/privkey.asc", "testdata/pubkey", pass, pointer.ToString("bc8acdd415bd80b3")},
	{"armored unprotected-with-key-id", "testdata/privkey_unprotected.asc", "testdata/pubkey", "", pointer.ToString("bc8acdd415bd80b3")},
	{"gpg subkey unprotected-with-key-id", "testdata/privkey_unprotected_subkey_only.asc", "testdata/pubkey", "", pointer.ToString("9890904dfb2ec88a")},
}

func TestPGPSignerAndVerify(t *testing.T) {
	data := []byte("testdata")
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			armoredPublicKey := fmt.Sprintf("%s.asc", testCase.pubKeyFile)
			gpgPublicKey := fmt.Sprintf("%s.gpg", testCase.pubKeyFile)
			sig, err := PGPSignerWithKeyID(testCase.privKeyFile, testCase.pass, testCase.keyID)(data)
			require.NoError(t, err)

			err = PGPVerify(bytes.NewReader(data), sig, armoredPublicKey)
			require.NoError(t, err)

			err = PGPVerify(bytes.NewReader(data), sig, gpgPublicKey)
			require.NoError(t, err)
			if testCase.keyID != nil {
				var pgpSignature *crypto.PGPSignature
				if isASCII(sig) {
					pgpSignature, err = crypto.NewPGPSignatureFromArmored(string(sig))
					require.NoError(t, err)
				} else {
					pgpSignature = crypto.NewPGPSignature(sig)
				}
				sigID, _ := pgpSignature.GetSignatureKeyIDs()
				require.Len(t, sigID, 1)
				require.Equal(t, *testCase.keyID, fmt.Sprintf("%x", sigID[0]))
			}
		})
	}
}

func TestArmoredDetachSignAndVerify(t *testing.T) {
	data := []byte("testdata")
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			armoredPublicKey := fmt.Sprintf("%s.asc", testCase.pubKeyFile)
			gpgPublicKey := fmt.Sprintf("%s.gpg", testCase.pubKeyFile)
			sig, err := PGPArmoredDetachSignWithKeyID(
				bytes.NewReader(data),
				testCase.privKeyFile,
				testCase.pass,
				testCase.keyID,
			)
			require.NoError(t, err)

			err = PGPVerify(bytes.NewReader(data), sig, armoredPublicKey)
			require.NoError(t, err)

			err = PGPVerify(bytes.NewReader(data), sig, gpgPublicKey)
			require.NoError(t, err)
			if testCase.keyID != nil {
				var pgpSignature *crypto.PGPSignature
				if isASCII(sig) {
					pgpSignature, err = crypto.NewPGPSignatureFromArmored(string(sig))
					require.NoError(t, err)
				} else {
					pgpSignature = crypto.NewPGPSignature(sig)
				}
				sigID, _ := pgpSignature.GetSignatureKeyIDs()
				require.Len(t, sigID, 1)
				require.Equal(t, *testCase.keyID, fmt.Sprintf("%x", sigID[0]))
			}
		})
	}
}

func TestPGPSignerError(t *testing.T) {
	_, err := PGPSigner("/does/not/exist", "")([]byte("data"))
	require.Error(t, err)

	var expectedError *nfpm.ErrSigningFailure
	require.True(t, errors.As(err, &expectedError))
}

func TestNoSigningKey(t *testing.T) {
	_, err := readSigningKey("testdata/pubkey.asc", pass)
	require.EqualError(t, err, "no signing key in keyring")
}

func TestMultipleKeys(t *testing.T) {
	_, err := readSigningKey("testdata/multiple_privkeys.asc", pass)
	require.EqualError(t, err, "more than one signing key in keyring")
}

func TestWrongPass(t *testing.T) {
	_, err := readSigningKey("testdata/privkey.asc", "password123")
	require.Contains(t, err.Error(), "private key checksum failure")
}

func TestEmptyPass(t *testing.T) {
	_, err := readSigningKey("testdata/privkey.asc", "")
	require.EqualError(t, err, "key is encrypted but no passphrase was provided")
}

func TestReadArmoredKey(t *testing.T) {
	_, err := readSigningKey("testdata/privkey.asc", pass)
	require.NoError(t, err)
}

func TestReadKey(t *testing.T) {
	_, err := readSigningKey("testdata/privkey.gpg", pass)
	require.NoError(t, err)
}

func TestIsASCII(t *testing.T) {
	data, err := os.ReadFile("testdata/privkey.asc")
	require.NoError(t, err)
	require.True(t, isASCII(data))

	data, err = os.ReadFile("testdata/privkey.gpg")
	require.NoError(t, err)
	require.False(t, isASCII(data))
}
