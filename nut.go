package sqrl

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"sync/atomic"
	"time"
)

// Nutter generates new nuts used to issue
// unique challenges to a SQRL client. It is
// also used to validate nuts that were
// previously issued.
type Nutter struct {
	Expiry time.Duration

	key    []byte
	aesgcm cipher.AEAD
}

// NewNutter creates a Nut generator
// with the given encryption key and a
// default nut expiry of 5 minutes.
// TODO: Key rotation
func NewNutter(key []byte) *Nutter {
	aesgcm := genAesgcm(key)
	return &Nutter{
		key:    key,
		aesgcm: aesgcm,
		Expiry: time.Minute * 5,
	}
}

func genAesgcm(key []byte) cipher.AEAD {
	padKeyIfRequired(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err.Error())
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}
	return aesgcm
}

func padKeyIfRequired(key []byte) {
	// TODO: Ensure key is either 16, 24 or 32 bits
}

// NoClientID is used to represent a nut that will not
// perform any client identification check when validated.
const NoClientID = ""

var noClientIDBytes = make([]byte, 4)

var nuts uint32

// Nut is a base64, encrypted nonce that contains
// metadata about the request that it was derived from.
type Nut string

func (n Nut) String() string {
	return string(n)
}

// Nut returns a challenge that should be returned to
// SQRL client for signing.
//
// The Nut (think nonce) is guaranteed to be unique
// and unpredictable to prevent replay attacks.
//
// clientIdentifier should at least include the IP address
// of the client the Nut is being generated for, but could
// include other intentification information such as User-Agent.
//
// It is important that clientIdentifier is created in a
// deterministic way, as it must match the clientIdentifier
// used during nut validation.
//
// Alternatively, NoClientID can be used to skip the client
// identification check. This should only be used if client
// identification is not possible.
func (n *Nutter) Nut(clientIdentifier string) Nut {
	//  32 bits: user's connection IP address if secured, 0.0.0.0 if non-secured.
	//  32 bits: UNIX-time timestamp incrementing once per second.
	//  32 bits: up-counter incremented once for every SQRL link generated.
	//  31 bits: pseudo-random noise from system source.
	//   1  bit: flag bit to indicate source: QRcode or URL click
	// ---------
	// 128 bits: AES encryption block size

	nut := make([]byte, 16)

	//  32 bits: user's connection IP address if secured, 0.0.0.0 if non-secured.
	ip := nutClientIDBytes(clientIdentifier)
	nut[0] = ip[0]
	nut[1] = ip[1]
	nut[2] = ip[2]
	nut[3] = ip[3]

	// UNIX-time timestamp incrementing once per second.
	t := uint32(time.Now().Unix())
	nut[4] = byte(t >> 24)
	nut[5] = byte(t >> 16)
	nut[6] = byte(t >> 8)
	nut[7] = byte(t)

	//  32 bits: up-counter incremented once for every SQRL link generated.
	// TODO combine this with a machine fingerprint
	count := atomic.AddUint32(&nuts, 1)
	nut[8] = byte(count >> 24)
	nut[9] = byte(count >> 16)
	nut[10] = byte(count >> 8)
	nut[11] = byte(count)

	//  31 bits: pseudo-random noise from system source.
	noise := randBytes(4)
	nut[12] = noise[0]
	nut[13] = noise[1]
	nut[14] = noise[2]
	nut[15] = noise[3]

	//   1  bit: flag bit to indicate source: QRcode or URL click
	// TODO

	nonce := randBytes(n.aesgcm.NonceSize())
	encryptedNut := n.aesgcm.Seal(nil, nonce, nut, nil)
	encryptedNutAndNonce := append(nonce, encryptedNut...)
	return Nut(Base64.EncodeToString(encryptedNutAndNonce))
}

// Validate checks a nut returned by a client to ensure the nut
// is valid.
//
// The clients identifier (usually IP) is checked against the
// identifier encrypted in the nut to ensure the nut has been
// returned from the same machine it was originally sent to.
//
// Note: The client ID check will not be performed if the nut was
// created with NoClientID.
//
// The nut's expiry is also checked, to ensure there hasn't been
// a significant delay between nut issuing and nut return.
func (n *Nutter) Validate(returned Nut, clientIdentifier string) bool {
	decryptedNut, err := n.decryptNut(returned)
	if err != nil || len(decryptedNut) != 16 {
		return false // TODO: Do we need to expose this error?
	}

	originalIP := decryptedNut[:4]
	shouldCheckIP := bytes.Equal(originalIP, noClientIDBytes)
	if !shouldCheckIP {
		ip := nutClientIDBytes(clientIdentifier)
		if ipMatch := bytes.Equal(ip, originalIP); !ipMatch {
			return false
		}
	}

	timeSeconds := binary.BigEndian.Uint32(decryptedNut[4:8])
	t := time.Unix(int64(timeSeconds), 0)
	return time.Since(t) <= n.Expiry
}

func (n *Nutter) decryptNut(encrypted Nut) ([]byte, error) {
	decodedNutAndNonce, err := Base64.DecodeString(string(encrypted))
	if err != nil {
		return nil, err
	}
	nonceSize := n.aesgcm.NonceSize()
	if len(decodedNutAndNonce) <= nonceSize {
		return nil, errors.New("invalid nut")
	}
	nonce := decodedNutAndNonce[:nonceSize]
	encryptedNut := decodedNutAndNonce[nonceSize:]

	return n.aesgcm.Open(nil, nonce, encryptedNut, nil)
}

func nutClientIDBytes(clientIdentifier string) []byte {
	if clientIdentifier == NoClientID {
		return noClientIDBytes
	}
	hashedClientID := md5.Sum([]byte(clientIdentifier))
	return hashedClientID[:4]
}

func randBytes(length int) []byte {
	noise := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, noise); err != nil {
		// Nut generation does not currently return
		// an error as there is little recourse available
		// to a consumer.
		// It is probably safe to assume that a failure
		// to read random noise is a non-recoverable error.
		// This is an assumption that should be tested.
		panic(err.Error())
	}
	return noise
}
