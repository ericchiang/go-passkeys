package packed

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/go-passkeys/go-passkeys/webauthn"
)

func TestVerifyAttestationPacked(t *testing.T) {
	testCases := []struct {
		name              string
		rp                *webauthn.RelyingParty
		challenge         string
		clientData        string
		attestationObject string
	}{
		{
			name: "YubiKey 5 Series",
			rp: &webauthn.RelyingParty{
				ID:     "localhost",
				Origin: "http://localhost:8080",
			},
			challenge:         "-ium4NdjLD6Acqy9p66NtA",
			clientData:        `{"type":"webauthn.create","challenge":"-ium4NdjLD6Acqy9p66NtA","origin":"http://localhost:8080","crossOrigin":false}`,
			attestationObject: "o2NmbXRmcGFja2VkZ2F0dFN0bXSjY2FsZyZjc2lnWEgwRgIhAL7ex0WTU1ZpLSRhoTxNxaYbwYcaNEA/h9eJEp0weJEqAiEA1vMTwi4bkvkE/gzQDO1seRyw0SupYth902MWOpZ0TDpjeDVjgVkC3TCCAtkwggHBoAMCAQICCQCkQGRCP4Vr/DANBgkqhkiG9w0BAQsFADAuMSwwKgYDVQQDEyNZdWJpY28gVTJGIFJvb3QgQ0EgU2VyaWFsIDQ1NzIwMDYzMTAgFw0xNDA4MDEwMDAwMDBaGA8yMDUwMDkwNDAwMDAwMFowbzELMAkGA1UEBhMCU0UxEjAQBgNVBAoMCVl1YmljbyBBQjEiMCAGA1UECwwZQXV0aGVudGljYXRvciBBdHRlc3RhdGlvbjEoMCYGA1UEAwwfWXViaWNvIFUyRiBFRSBTZXJpYWwgMTExMzg2NjQwNDBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABPkOtta+hbyNLleVf1puWkTqbHzBJz+y42wVbN881zPGfYHty7riyxT4c3fcoXK+bl1/XE7f/2D3I3WT9ILQVYOjgYEwfzATBgorBgEEAYLECg0BBAUEAwUHATAiBgkrBgEEAYLECgIEFTEuMy42LjEuNC4xLjQxNDgyLjEuNzATBgsrBgEEAYLlHAIBAQQEAwIFIDAhBgsrBgEEAYLlHAEBBAQSBBAZCDw9g4NLGLwDjxyasv0bMAwGA1UdEwEB/wQCMAAwDQYJKoZIhvcNAQELBQADggEBAHzCOWZTA+e+ni1+kmfydBAZgdLyWGbYLQxlJtjd00qbh6M41UaYuRm12eKm3uYDgPT1BnVqqGN69k/1+P91O+knuRBfb48El12Up1hfzyON1UKGgBA6IdmghqYbK+X5baMMLGdsZ1nLKEWjVRecjLg79GwHy9HJ25j+Gb7+yNZMJdfgMJvfrecD35Tgmw+3fTCbzpnlW9Sp/LNdkHjdECaicue3MdhtrwaVmNfyVNvU5mqHzQAH2zf4/TsTZKdx2aIDFmqZZAartwD7RskFfQpnN0CWU6uCaBS0ECgDPLLW3q39mfvJ/y2rHPhaSWue85+2lNK+NJPP43ZsNrA7Rw5oYXV0aERhdGFYwkmWDeWIDoxodDQXD2R2YFuP5K65ooYyx5lc87qDHZdjxQAAAAMZCDw9g4NLGLwDjxyasv0bADDC4gNtuVFFZvyU4A2YDTFDSAOHTXQfTVUeXPpK2xTdoFx6LnSx3o2dcheLtBrEj0ylAQIDJiABIVggwuIDbblRRWb8lOANmAK3w9dppoKQXC2rw7yY6c9W/C4iWCBp5XU3NpH55RWYheccEtji/4Yc+zscmwMQN+KrQ/o7/qFrY3JlZFByb3RlY3QD",
		},
		{
			name: "Chrome local",
			rp: &webauthn.RelyingParty{
				ID:     "localhost",
				Origin: "http://localhost:8080",
			},
			challenge:         "8XJI5cQqW-VqtSPO7JIpUg",
			clientData:        `{"type":"webauthn.create","challenge":"8XJI5cQqW-VqtSPO7JIpUg","origin":"http://localhost:8080","crossOrigin":false}`,
			attestationObject: "o2NmbXRmcGFja2VkZ2F0dFN0bXSiY2FsZyZjc2lnWEcwRQIhAJdhPjKXQAoWBgBDw+tu8q2WpTrXLULwFBgpJGu0SLI7AiA493f+tIVJkf9oeSX24FsSHJqkNKYmph2IAD7wSzTMAGhhdXRoRGF0YVikSZYN5YgOjGh0NBcPZHZgW4/krrmihjLHmVzzuoMdl2NFAAAAAK3OAAI1vMYKZIsLJfHwVQMAIGfNA5n4RSq0gsGzIB6kmazzLLe0goRP+1QG4uixw+zTpQECAyYgASFYIJtUv3C9FxTn1i7xALbGQJjzDkyFECHaHQ5+KYom9eh9IlggCfXDLnVZU9KEKuhqdPInGHcfAlZSCTOeRWSUzrSkkHo=",
		},
	}

	blobData, err := os.ReadFile("testdata/blob.jwt")
	if err != nil {
		t.Fatalf("loading metadata blob: %v", err)
	}
	parts := strings.Split(string(blobData), ".")
	if len(parts) != 3 {
		t.Fatalf("Failed to parse blob JWT, expected 3 parts got %d", len(parts))
	}
	mdRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("Failed to decode JWT: %v", err)
	}

	md := &metadata{}
	if err := json.Unmarshal(mdRaw, md); err != nil {
		t.Fatalf("parsing metadata blob: %v", err)
	}
	opts := &VerifyOptions{
		GetRoots:          md.getRoots,
		AllowSelfAttested: true,
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			challenge, err := base64.RawURLEncoding.DecodeString(tc.challenge)
			if err != nil {
				t.Fatalf("Parsing challenge: %v", err)
			}
			attestationObject, err := base64.StdEncoding.DecodeString(tc.attestationObject)
			if err != nil {
				t.Fatalf("Parsing attestation object: %v", err)
			}
			clientDataJSON := []byte(tc.clientData)
			attObj, err := tc.rp.VerifyAttestationObject(challenge, clientDataJSON, attestationObject)
			if err != nil {
				t.Fatalf("Verifying attestation object: %v", err)
			}
			if _, err := Verify(attObj, tc.rp.ID, clientDataJSON, opts); err != nil {
				t.Errorf("Verifying attestation: %v", err)
			}
		})
	}
}

// metadata is a parsed FIDO metadata Service BLOB, and can be used to validate
// the certificate chain of "packed" attestations.
//
// https://fidoalliance.org/metadata/
type metadata struct {
	Entries []*metadataEntry `json:"entries"`
}

func (m *metadata) getRoots(aaguid webauthn.AAGUID) (*x509.CertPool, error) {
	for _, ent := range m.Entries {
		if ent.Metadata.AAGUID != aaguid {
			continue
		}

		pool := x509.NewCertPool()
		for _, cert := range ent.Metadata.AttestationRootCertificates {
			data, err := base64.StdEncoding.DecodeString(cert)
			if err != nil {
				return nil, fmt.Errorf("decoding certificate base64 for aaguid %s: %v", aaguid, err)
			}
			cert, err := x509.ParseCertificate(data)
			if err != nil {
				return nil, fmt.Errorf("parsing certificate for aaguid %s: %v", aaguid, err)
			}
			pool.AddCert(cert)
		}
		return pool, nil
	}
	return nil, fmt.Errorf("no certificates found for aaguid: %s", aaguid)
}

// https://fidoalliance.org/specs/mds/fido-metadata-service-v3.0-ps-20210518.html
type metadataEntry struct {
	AAID     string            `json:"aaid"`
	AAGUID   webauthn.AAGUID   `json:"aaguid"`
	KeyIDs   []string          `json:"attestationCertificateKeyIdentifiers"`
	Metadata metadataStatement `json:"metadataStatement"`
}

// https://fidoalliance.org/specs/mds/fido-metadata-statement-v3.0-ps-20210518.html#metadata-keys
type metadataStatement struct {
	AAGUID                      webauthn.AAGUID `json:"aaguid"`
	Description                 string          `json:"description"`
	AttestationRootCertificates []string        `json:"attestationRootCertificates"`
}
