package crypto

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
)

// mockEntropySource is a mock implementation of entropySource that allows the
// client to specify the returned entropy and error.
type mockEntropySource struct {
	called  bool
	entropy [EntropySize]byte
	err     error
}

func (es *mockEntropySource) getEntropy() (entropy [EntropySize]byte, err error) {
	es.called = true
	return es.entropy, es.err
}

// mockKeyDeriver is a mock implementation of keyDeriver that saves its provided
// entropy and allows the client to specify the returned SecretKey and
// PublicKey.
type mockKeyDeriver struct {
	called  bool
	entropy [EntropySize]byte
	sk      SecretKey
	pk      PublicKey
}

func (kd *mockKeyDeriver) deriveKeyPair(entropy [EntropySize]byte) (sk SecretKey, pk PublicKey) {
	kd.called = true
	kd.entropy = entropy
	return kd.sk, kd.pk
}

// Test that the Generate method is properly calling its dependencies and
// returning the expected key pair.
func TestGenerateRandomKeyPair(t *testing.T) {
	var mockEntropy [EntropySize]byte
	mockEntropy[0] = 0x0a
	mockEntropy[EntropySize-1] = 0x0b
	es := mockEntropySource{entropy: mockEntropy}

	sk := SecretKey([SecretKeySize]byte{})
	pk := PublicKey([PublicKeySize]byte{})
	kd := mockKeyDeriver{sk: sk, pk: pk}

	// Create a SignatureKeyGenerator using mocks.
	g := stdGenerator{&es, &kd}

	// Create key pair.
	skActual, pkActual, err := g.Generate()

	// Verify that we got back the expected results.
	if err != nil {
		t.Error(err)
	}
	if sk != skActual {
		t.Errorf("Generated secret key does not match expected! expected = %v, actual = %v", sk, skActual)
	}
	if pk != pkActual {
		t.Errorf("Generated public key does not match expected! expected = %v, actual = %v", pk, pkActual)
	}

	// Verify the dependencies were called correctly
	if !es.called {
		t.Error("entropySource was never called.")
	}
	if !kd.called {
		t.Error("keyDeriver was never called.")
	}
	if mockEntropy != kd.entropy {
		t.Error("keyDeriver was called with the wrong entropy. expected = %v, actual = %v", mockEntropy, kd.entropy)
	}
}

// Test that the Generate method fails if the call to entropy source fails
func TestGenerateRandomKeyPairFailsWhenRandFails(t *testing.T) {
	es := mockEntropySource{err: errors.New("mock error from entropy source")}
	g := stdGenerator{es: &es}
	if _, _, err := g.Generate(); err == nil {
		t.Error("Generate should fail when entropy source fails.")
	}
}

// Test that the GenerateDeterministic method is properly calling its
// dependencies and returning the expected key pair.
func TestGenerateDeterministicKeyPair(t *testing.T) {
	// Create entropy bytes, setting a few bytes explicitly instead of using a
	// buffer of random bytes.
	var mockEntropy [EntropySize]byte
	mockEntropy[0] = 0x0a
	mockEntropy[EntropySize-1] = 0x0b

	sk := SecretKey([SecretKeySize]byte{})
	pk := PublicKey([PublicKeySize]byte{})
	kd := mockKeyDeriver{sk: sk, pk: pk}
	g := stdGenerator{kd: &kd}

	// Create key pair.
	skActual, pkActual := g.GenerateDeterministic(mockEntropy)

	// Verify that we got back the right results.
	if sk != skActual {
		t.Errorf("Generated secret key does not match expected! expected = %v, actual = %v", sk, skActual)
	}
	if pk != pkActual {
		t.Errorf("Generated public key does not match expected! expected = %v, actual = %v", pk, pkActual)
	}

	// Verify the dependencies were called correctly.
	if !kd.called {
		t.Error("keyDeriver was never called.")
	}
	if mockEntropy != kd.entropy {
		t.Error("keyDeriver was called with the wrong entropy. expected = %v, actual = %v", mockEntropy, kd.entropy)
	}
}

// Creates and encodes a public key, and verifies that it decodes correctly,
// does the same with a signature.
func TestSignatureEncoding(t *testing.T) {
	// Create a dummy key pair.
	var sk SecretKey
	sk[0] = 0x0a
	sk[32] = 0x0b
	pk := sk.PublicKey()

	// Marshal and unmarshal the public key.
	marshalledPK := encoding.Marshal(pk)
	var unmarshalledPK PublicKey
	err := encoding.Unmarshal(marshalledPK, &unmarshalledPK)
	if err != nil {
		t.Fatal(err)
	}

	// Test the public keys for equality.
	if pk != unmarshalledPK {
		t.Error("pubkey not the same after marshalling and unmarshalling")
	}

	// Create a signature using the secret key.
	var signedData Hash
	rand.Read(signedData[:])
	sig, err := SignHash(signedData, sk)
	if err != nil {
		t.Fatal(err)
	}

	// Marshal and unmarshal the signature.
	marshalledSig := encoding.Marshal(sig)
	var unmarshalledSig Signature
	err = encoding.Unmarshal(marshalledSig, &unmarshalledSig)
	if err != nil {
		t.Fatal(err)
	}

	// Test signatures for equality.
	if sig != unmarshalledSig {
		t.Error("signature not same after marshalling and unmarshalling")
	}

}

// TestSigning creates a bunch of keypairs and signs random data with each of
// them.
func TestSigning(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Try a bunch of signatures because at one point there was a library that
	// worked around 98% of the time. Tests would usually pass, but 200
	// iterations would normally cause a failure.
	iterations := 200
	for i := 0; i < iterations; i++ {
		// Create dummy key pair.
		var entropy [EntropySize]byte
		entropy[0] = 0x05
		entropy[1] = 0x08
		sk, pk := StdKeyGen.GenerateDeterministic(entropy)

		// Generate and sign the data.
		var randData Hash
		rand.Read(randData[:])
		sig, err := SignHash(randData, sk)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the signature.
		err = VerifyHash(randData, pk, sig)
		if err != nil {
			t.Fatal(err)
		}

		// Attempt to verify after the data has been altered.
		randData[0] += 1
		err = VerifyHash(randData, pk, sig)
		if err != ErrInvalidSignature {
			t.Fatal(err)
		}

		// Restore the data and make sure the signature is valid again.
		randData[0] -= 1
		err = VerifyHash(randData, pk, sig)
		if err != nil {
			t.Fatal(err)
		}

		// Attempt to verify after the signature has been altered.
		sig[0] += 1
		err = VerifyHash(randData, pk, sig)
		if err != ErrInvalidSignature {
			t.Fatal(err)
		}
	}
}
