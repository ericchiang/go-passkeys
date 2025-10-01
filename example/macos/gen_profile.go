package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"time"

	"github.com/go-passkeys/go-passkeys/example/macos/internal/plist"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

func newUUID() string {
	var uuid [16]byte
	if _, err := io.ReadFull(rand.Reader, uuid[:]); err != nil {
		panic(err)
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant is 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

func newPKCS12Profile() ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generating key: %v", err)
	}
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, errors.New("failed to generate serial number: " + err.Error())
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: "Passkey testing"},
		SignatureAlgorithm:    x509.SHA256WithRSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 356),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %v", err)
	}

	data, err := pkcs12.Encode(rand.Reader, key, cert, nil, "password123")
	if err != nil {
		return nil, fmt.Errorf("encoding PKCS12: %v", err)
	}
	return data, nil
}

func main() {
	profile, err := newPKCS12Profile()
	if err != nil {
		log.Fatalf("Generating PKCS12 profile: %v", err)
	}

	keyUUID := newUUID()
	o := plist.Dict().
		Add("PayloadContent", plist.Array(
			plist.Dict().
				Add("PayloadType", plist.String("com.apple.security.pkcs12")).
				Add("PayloadUUID", plist.String(keyUUID)).
				Add("PayloadDisplayName", plist.String("CertificatePKCS12")).
				Add("Password", plist.String("password123")).
				Add("PayloadContent", plist.Data(profile)).
				Add("PayloadIdentifier", plist.String("go-passkeys.oblique.security.cert.pkcs12")).
				Add("PayloadVersion", plist.Int(1)),
			plist.Dict().
				Add("PayloadType", plist.String("com.apple.configuration.security.passkey.attestation")).
				Add("PayloadUUID", plist.String(newUUID())).
				Add("PayloadDisplayName", plist.String("PasskeyAttestation")).
				Add("AttestationIdentityAssetReference", plist.String("Passkey testing")).
				Add("RelyingParties", plist.Array(
					plist.String("localhost:8080"),
					plist.String("localhost"),
					plist.String("https://localhost:8080"),
				)).
				Add("PayloadIdentifier", plist.String("go-passkeys.oblique.security.passkey.attestation")).
				Add("PayloadVersion", plist.Int(1)),
		)).
		Add("PayloadDisplayName", plist.String("go-passkeys")).
		Add("PayloadIdentifier", plist.String("go-passkeys.oblique.security")).
		Add("PayloadType", plist.String("Configuration")).
		Add("PayloadUUID", plist.String(newUUID())).
		Add("PayloadVersion", plist.Int(1))
	data, err := plist.Marshal(o)
	if err != nil {
		log.Fatalf("Marshalling plist: %v", err)
	}
	fmt.Println(string(data))
}
