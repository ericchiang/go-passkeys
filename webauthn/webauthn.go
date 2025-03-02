package webauthn

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-passkeys/go-passkeys/webauthn/internal/cbor"
)

// Algorithm by the key to sign values, both a public key scheme and associated
// hashing function.
//
// https://www.w3.org/TR/webauthn-3/#typedefdef-cosealgorithmidentifier
type Algorithm int

// The set of algorithms recognized and supported by this package.
//
// https://www.iana.org/assignments/cose/cose.xhtml#algorithms
const (
	ES256 Algorithm = -7
	ES384 Algorithm = -35
	ES512 Algorithm = -36
	EdDSA Algorithm = -8
	RS256 Algorithm = -257
	RS384 Algorithm = -258
	RS512 Algorithm = -259
)

var algStrings = map[Algorithm]string{
	ES256: "ES256",
	ES384: "ES384",
	ES512: "ES512",
	EdDSA: "EdDSA",
	RS256: "RS256",
	RS384: "RS384",
	RS512: "RS512",
}

// Algorithm returns a human readable representation of the algorithm.
func (a Algorithm) String() string {
	if s, ok := algStrings[a]; ok {
		return s
	}
	return fmt.Sprintf("Algorithm(0x%x)", int(a))
}

// AttestationObject holds raw fields parsed from the attestationObject. This
// Is exported to support external validation of different attestation formats,
// and can generally be ignored by most package consumers.
//
// Standard WebAuthn use cases, which generally don't validate the trust chain of
// client credentials, should avoid using this type. Use
// [RelyingParty.VerifyAttestation] instead.
type AttestationObject struct {
	// Format of the attestation statement. Specification defined values include
	// "packed", "tpm", "android-key", "android-safetynet", "fido-u2f", "none",
	// "apple", and "compound".
	//
	// https://www.w3.org/TR/webauthn-3/#sctn-defined-attestation-formats
	Format string

	// AttestationStatement is the unparsed attStmt.
	AttestationStatement []byte

	// AuthenticatorData is the unparsed authData.
	AuthenticatorData []byte
}

// RelyingParty represents a server that attempts to validate webauthn
// credentials to authenticate users.
type RelyingParty struct {
	// The relying party identifier is a string that uniquely identifies the server.
	// This defaults to the "effective domain" of the origin. For example
	// "login.example.com".
	//
	// https://www.w3.org/TR/webauthn-3/#relying-party-identifier
	ID string

	// Origin is the base URL used by the browser when registering or challenging
	// a credential. For example "https://login.example.com:8080"
	Origin string
}

// VerifyAttestation validates a credential creation attempt. attestationObject
// and clientDataJSON arguments correspond directly to the credential response
// fields returned during creation. Challenge is the value passed to the creation
// call used to prevent replay attacks.
func (rp *RelyingParty) VerifyAttestation(challenge, clientDataJSON, attestationObject []byte) (*AuthenticatorData, error) {
	var clientData clientData
	if err := json.Unmarshal(clientDataJSON, &clientData); err != nil {
		return nil, fmt.Errorf("parsing client data: %v", err)
	}
	if clientData.Type != "webauthn.create" {
		return nil, fmt.Errorf("invalid client data type, expected 'webauthn.create', got '%s'", clientData.Type)
	}
	if clientData.Origin != rp.Origin {
		return nil, fmt.Errorf("invalid client data origin, expected '%s', got '%s'", rp.Origin, clientData.Origin)
	}
	if !clientData.Challenge.Equal(challenge) {
		return nil, fmt.Errorf("invalid client data challenge")
	}

	attObj, err := parseAttestationObject(attestationObject)
	if err != nil {
		return nil, fmt.Errorf("parsing attestation object: %v", err)
	}

	data, err := ParseAuthenticatorData(rp.ID, attObj.AuthenticatorData)
	if err != nil {
		return nil, fmt.Errorf("parsing authenticator data: %v", err)
	}
	return data, nil
}

// VerifyAttestationObject is like [RelyingParty.VerifyAttestation], but returns
// an unparsed attestation object, rather than validated authenticator data.
// This function is exported to support external validation of attestation
// statements, and shouldn't be used by most consumers.
func (rp *RelyingParty) VerifyAttestationObject(challenge, clientDataJSON, attestationObject []byte) (*AttestationObject, error) {
	var clientData clientData
	if err := json.Unmarshal(clientDataJSON, &clientData); err != nil {
		return nil, fmt.Errorf("parsing client data: %v", err)
	}
	if clientData.Type != "webauthn.create" {
		return nil, fmt.Errorf("invalid client data type, expected 'webauthn.create', got '%s'", clientData.Type)
	}
	if clientData.Origin != rp.Origin {
		return nil, fmt.Errorf("invalid client data origin, expected '%s', got '%s'", rp.Origin, clientData.Origin)
	}
	if !clientData.Challenge.Equal(challenge) {
		return nil, fmt.Errorf("invalid client data challenge")
	}

	attObj, err := parseAttestationObject(attestationObject)
	if err != nil {
		return nil, fmt.Errorf("parsing attestation object: %v", err)
	}
	return attObj, nil
}

// VerifyAssertion validates an authentication assertion. The public key
// and algorithm should use the [AuthenticatorData] values for the credential.
// The challenge is the value passed to the frontend to sign. authenticatorData,
// clientDataJSON, and signature should be the values returned by the credential
// assertion.
func (rp *RelyingParty) VerifyAssertion(pub crypto.PublicKey, alg Algorithm, challenge, clientDataJSON, authData, sig []byte) (*Assertion, error) {
	clientDataHash := sha256.Sum256(clientDataJSON)

	var clientData clientData
	if err := json.Unmarshal(clientDataJSON, &clientData); err != nil {
		return nil, fmt.Errorf("parsing client data: %v", err)
	}
	if clientData.Type != "webauthn.get" {
		return nil, fmt.Errorf("invalid client data type, expected 'webauthn.get', got '%s'", clientData.Type)
	}
	if clientData.Origin != rp.Origin {
		return nil, fmt.Errorf("invalid client data origin, expected '%s', got '%s'", rp.Origin, clientData.Origin)
	}
	if !clientData.Challenge.Equal(challenge) {
		return nil, fmt.Errorf("invalid client data challenge")
	}

	data := append([]byte{}, authData...)
	data = append(data, clientDataHash[:]...)
	if err := VerifySignature(pub, alg, data, sig); err != nil {
		return nil, fmt.Errorf("invalid signature: %v", err)
	}

	rpIDHash := sha256.Sum256([]byte(rp.ID))
	if len(authData) < 32 {
		return nil, fmt.Errorf("not enough bytes for rpid hash")
	}
	if !bytes.Equal(rpIDHash[:], authData[:32]) {
		return nil, fmt.Errorf("assertion issued for different relying party")
	}
	if len(authData) < 32+1 {
		return nil, fmt.Errorf("not enough bytes for flag")
	}
	flags := Flags(authData[32])
	if len(authData) < 32+1+4 {
		return nil, fmt.Errorf("not enough bytes for counter")
	}

	counter := binary.BigEndian.Uint32(authData[32+1 : 32+1+4])
	return &Assertion{
		Flags:   flags,
		Counter: counter,
	}, nil
}

// VerifySignature is a low-level API used to validate raw signatures for a
// given COSE algorithm. This is exported to support external attestation
// statement validators. To validate a WebAuthn authentication event, use
// [RelyingParty.VerifyAssertion] instead.
func VerifySignature(pub crypto.PublicKey, alg Algorithm, data, sig []byte) error {
	switch alg {
	case ES256:
		ecdsaPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("invalid public key type for ES256 algorithm: %T", pub)
		}
		h := sha256.New()
		h.Write(data)
		if !ecdsa.VerifyASN1(ecdsaPub, h.Sum(nil), sig) {
			return fmt.Errorf("invalid ES256 signature")
		}
	case ES384:
		ecdsaPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("invalid public key type for ES384 algorithm: %T", pub)
		}
		h := sha512.New384()
		h.Write(data)
		if !ecdsa.VerifyASN1(ecdsaPub, h.Sum(nil), sig) {
			return fmt.Errorf("invalid ES384 signature")
		}
	case ES512:
		ecdsaPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("invalid public key type for ES512 algorithm: %T", pub)
		}
		h := sha512.New()
		h.Write(data)
		if !ecdsa.VerifyASN1(ecdsaPub, h.Sum(nil), sig) {
			return fmt.Errorf("invalid ES512 signature")
		}
	case EdDSA:
		ed25519Pub, ok := pub.(*ed25519.PublicKey)
		if !ok {
			return fmt.Errorf("invalid public key type for EdDSA algorithm: %T", pub)
		}
		if !ed25519.Verify(*ed25519Pub, data, sig) {
			return fmt.Errorf("invalid EdDSA signature")
		}
	case RS256:
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("invalid public key type for RSA256 algorithm: %T", pub)
		}
		h := sha256.New()
		h.Write(data)
		if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, h.Sum(nil), sig); err != nil {
			return fmt.Errorf("invalid RS256 signature: %v", err)
		}
	case RS384:
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("invalid public key type for RSA384 algorithm: %T", pub)
		}
		h := sha512.New384()
		h.Write(data)
		if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA384, h.Sum(nil), sig); err != nil {
			return fmt.Errorf("invalid RS384 signature: %v", err)
		}
	case RS512:
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("invalid public key type for RSA512 algorithm: %T", pub)
		}
		h := sha512.New()
		h.Write(data)
		if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA512, h.Sum(nil), sig); err != nil {
			return fmt.Errorf("invalid RS512 signature: %v", err)
		}
	default:
		return fmt.Errorf("unsupported signing algorithm: %d", alg)
	}
	return nil
}

// Flags represents authenticator data flags, providing information such as the
// sync state of a credential.
//
// https://www.w3.org/TR/webauthn-3/#authdata-flags
type Flags byte

// String returns a human readable representation of the flags.
func (f Flags) String() string {
	var vals []string
	if f.UserPresent() {
		vals = append(vals, "UP")
	}
	if (byte(f) & (1 << 1)) != 0 {
		vals = append(vals, "RFU1")
	}
	if f.UserVerified() {
		vals = append(vals, "UV")
	}
	if f.BackupEligible() {
		vals = append(vals, "BE")
	}
	if f.BackedUp() {
		vals = append(vals, "BS")
	}
	if (byte(f) & (1 << 5)) != 0 {
		vals = append(vals, "RFU2")
	}
	if f.AttestedCredentialData() {
		vals = append(vals, "AT")
	}
	if f.Extensions() {
		vals = append(vals, "ED")
	}
	if len(vals) == 0 {
		return "Flags()"
	}
	return fmt.Sprintf("Flags(%s)", strings.Join(vals, "|"))
}

// UserPresent identifies if the authenticator performed a successfull user
// presence test.
//
// https://www.w3.org/TR/webauthn-3/#concept-user-present
func (f Flags) UserPresent() bool {
	return (byte(f) & 1) != 0
}

// UserVerified identifies if an authenticator performed additional authorization
// of a creation or authentication event, such as a password entry or biometric
// challenge.
//
// https://www.w3.org/TR/webauthn-3/#concept-user-verified
func (f Flags) UserVerified() bool {
	return (byte(f) & (1 << 2)) != 0
}

// BackupEligible identifies if a credential can be backed up to external storage
// (such as a passkey), or if the credential is single-device.
//
// https://www.w3.org/TR/webauthn-3/#backup-eligible
func (f Flags) BackupEligible() bool {
	return (byte(f) & (1 << 3)) != 0
}

// BackedUp identifies if a credential has been synced to external storage.
//
// https://www.w3.org/TR/webauthn-3/#backed-up
func (f Flags) BackedUp() bool {
	return (byte(f) & (1 << 4)) != 0
}

// AttestedCredentialData identifies if a credential contains an attestatino
// statement.
//
// https://www.w3.org/TR/webauthn-3/#attested-credential-data
func (f Flags) AttestedCredentialData() bool {
	return (byte(f) & (1 << 6)) != 0
}

// Extensions identifies if the authenticator data contains additional extensions.
//
// https://www.w3.org/TR/webauthn-3/#authdata-extensions
func (f Flags) Extensions() bool {
	return (byte(f) & (1 << 7)) != 0
}

// Assertion holds subsets of the information provided by the
// authenticator and used during signing.
//
// https://www.w3.org/TR/webauthn-3/#sctn-authenticator-data
type Assertion struct {
	// Various bits of information about this key, such as if it is synced to a
	// Cloud service.
	//
	// https://www.w3.org/TR/webauthn-3/#authdata-flags
	Flags Flags
	// Counter is incremented value that is increased every time the key signs a
	// challenge. This may be zero for authenticators that don't support signing
	// counters.
	//
	// Signature counters are intended to be used to detect cloned credentials,
	// but are generally unsupported by keys synced across multipled devices.
	//
	// https://www.w3.org/TR/webauthn-3/#sctn-sign-counter
	Counter uint32
}

// AuthenticatorData holds information about an individual credential. This data is
// provided by the browser within the context of the origin registering the
// key. In some circumstances, can be attested to be resident on a physical
// security key or device.
//
// https://www.w3.org/TR/webauthn-3/#authenticator-data
type AuthenticatorData struct {
	// Various bits of information about this key, such as if it is synced to a
	// Cloud service.
	//
	// https://www.w3.org/TR/webauthn-3/#authdata-flags
	Flags Flags
	// Counter is incremented value that is increased every time the key signs a
	// challenge. This may be zero for authenticators that don't support signing
	// counters.
	//
	// Signature counters are intended to be used to detect cloned credentials,
	// but are generally unsupported by keys synced across multipled devices.
	//
	// https://www.w3.org/TR/webauthn-3/#sctn-sign-counter
	Counter uint32

	// The identifier for authenticator or service that stores the credential.
	//
	// [AAGUID.Name] can be used to translate this to a human readable string, such
	// as "iCloud Keychain" or "Google Password Manager".
	AAGUID AAGUID

	// Raw ID of the credential, generated by the authenticator.
	//
	// This value is used during authentication to identify which keys are being
	// challenged, and during registration to avoid re-registering the same key
	// twice.
	//
	// https://www.w3.org/TR/webauthn-3/#credential-id
	// https://www.w3.org/TR/webauthn-3/#dom-publickeycredentialcreationoptions-excludecredentials
	// https://www.w3.org/TR/webauthn-3/#dom-publickeycredentialrequestoptions-allowcredentials
	CredentialID []byte

	// Algorithm used by the key to sign challenges.
	Algorithm Algorithm
	// Public key parse from the attestation statement.
	//
	// Callers can use [x509.MarshalPKIXPublicKey] and [x509.ParsePKIXPublicKey] to
	// serialize this value.
	PublicKey crypto.PublicKey

	// Raw extension data.
	Extensions []byte
}

// parseAttestationObject parses the result of a key creation event. This
// includes information such as the public key, key ID, RP ID hash, etc.
//
//	const cred = await navigator.credentials.create({
//		publicKey: {
//			// ...
//		},
//	});
//	console.log(cred.response.attestationObject);
//
// https://www.w3.org/TR/webauthn-3/#attestation-object
func parseAttestationObject(b []byte) (*AttestationObject, error) {
	d := cbor.NewDecoder(b)
	var (
		format   string
		authData []byte
		attest   []byte
	)
	if !d.Map(func(kv *cbor.Decoder) bool {
		var key string
		if !kv.String(&key) {
			return false
		}
		switch key {
		case "fmt":
			return kv.String(&format)
		case "attStmt":
			return kv.Raw(&attest)
		case "authData":
			return kv.Bytes(&authData)
		default:
			return kv.Skip()
		}
	}) || !d.Done() {
		return nil, fmt.Errorf("invalid cbor data")
	}
	if len(authData) == 0 {
		return nil, fmt.Errorf("no auth data")
	}
	return &AttestationObject{
		Format:               format,
		AttestationStatement: attest,
		AuthenticatorData:    authData,
	}, nil
}

// ParseAuthenticatorData parses authData into its various fields.  This
// function is exported to support external validation of attestation
// statements. Most package consumers should derive a AuthenticatorData from
// [RelyingParty.VerifyAttestation] instead
func ParseAuthenticatorData(rpID string, b []byte) (*AuthenticatorData, error) {
	var ad AuthenticatorData
	if len(b) < 32 {
		return nil, fmt.Errorf("not enough bytes for rpid hash")
	}

	var rpidHash [32]byte
	copy(rpidHash[:], b[:32])
	wantRPID := sha256.Sum256([]byte(rpID))
	if wantRPID != rpidHash {
		return nil, fmt.Errorf("authenticator data doesn't match relying party ID")
	}

	b = b[32:]
	if len(b) < 1 {
		return nil, fmt.Errorf("not enough bytes for flag")
	}
	ad.Flags = Flags(b[0])
	b = b[1:]
	if len(b) < 4 {
		return nil, fmt.Errorf("not enough bytes for counter")
	}

	ad.Counter = binary.BigEndian.Uint32(b[:4])
	b = b[4:]

	if len(b) < 16 {
		return nil, fmt.Errorf("not enough bytes for aaguid")
	}
	copy(ad.AAGUID[:], b[:16])
	b = b[16:]

	if len(b) < 2 {
		return nil, fmt.Errorf("not enough bytes for cred ID length")
	}
	credIDSize := binary.BigEndian.Uint16(b[:2])
	b = b[2:]

	size := int(credIDSize)
	if len(b) < size {
		return nil, fmt.Errorf("not enough bytes for cred ID")
	}
	ad.CredentialID = b[:size]
	b = b[size:]

	d := cbor.NewDecoder(b)
	pub, err := d.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("parsing public key: %v", err)
	}
	ad.Algorithm = Algorithm(pub.Algorithm)
	ad.PublicKey = pub.Public
	if !d.Done() {
		ad.Extensions = d.Rest()
	}
	return &ad, nil
}

// clientDataChallenge is a wrapper on top of a WebAuthn challenge.
//
// Note that the specification recommends that "Challenges SHOULD therefore be
// at least 16 bytes long."
//
// https://www.w3.org/TR/webauthn-3/#sctn-cryptographic-challenges
type clientDataChallenge []byte

// Equal compares the challenge value against a set of bytes.
func (c clientDataChallenge) Equal(b []byte) bool {
	return subtle.ConstantTimeCompare([]byte(c), b) == 1
}

// UnmarshalJSON implements the challenge encoding used by clientDataJSON.
//
// https://www.w3.org/TR/webauthn-3/#dom-authenticatorresponse-clientdatajson
func (c *clientDataChallenge) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("challenge value doesn't parse into string: %v", err)
	}
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	*c = clientDataChallenge(data)
	return nil
}

// clientData holds information passed to the authenticator for both registration
// and authentication.
//
// https://www.w3.org/TR/webauthn-3/#dictionary-client-data
//
// JSON tags are added to provide unmarshalling from the clientDataJSON format.
//
// https://www.w3.org/TR/webauthn-3/#dom-authenticatorresponse-clientdatajson
type clientData struct {
	Type        string              `json:"type"`
	Challenge   clientDataChallenge `json:"challenge"`
	Origin      string              `json:"origin"`
	TopOrigin   string              `json:"topOrigin"`
	CrossOrigin bool                `json:"crossOrigin"`
}
