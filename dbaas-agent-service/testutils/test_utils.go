package testutils

import (
	"bytes"
	cryptoRand "crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	_ "github.com/netcracker/qubership-core-dbaas-agent/dbaas-agent-service/v2/config"
)

const (
	issuerFormat            = "%s/auth/realms/%s"
	wellknownEndpointFormat = "auth/realms/%s/.well-known/openid-configuration"
	certsEndpointFormat     = "auth/realms/%s/protocol/openid-connect/certs"
)

var (
	keys        map[string]*rsa.PrivateKey
	mainKeyId   string
	realm       string
	mockIdpHost string
	logger      logging.Logger
)

type CertResponse struct {
	Keys []CertResponseKey `json:"keys"`
}

// CertResponseKey is returned by the certs endpoint
type CertResponseKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type OidcResponse struct {
	JwksUri string `json:"jwks_uri"`
}

func init() {
	rand.Seed(time.Now().UnixNano())
	realm = "test-realm"
	logger = logging.GetLogger("security-test-utils")
	mainKeyId = generateKid()
	privateKey := generatePrivateKey()
	keys = make(map[string]*rsa.PrivateKey, 1)
	keys[mainKeyId] = privateKey
}

func generateKid() string {
	kid := make([]byte, 10)
	rand.Read(kid)
	return hex.EncodeToString(kid)
}

func generatePrivateKey() *rsa.PrivateKey {
	privateKey, _ := rsa.GenerateKey(cryptoRand.Reader, 2048)
	return privateKey
}

func SignToken(token *jwt.Token, kid string) (string, error) {
	token.Header["kid"] = kid
	return token.SignedString(keys[kid])
}

func issuer() string {
	if mockIdpHost == "" {
		mockIdpHost = GetMockServerUrl()
	}
	return fmt.Sprintf(issuerFormat, mockIdpHost, realm)
}

func GetTestToken(principal string, roles []string) *jwt.Token {
	return GetTestTokenWithClaims(principal, roles, nil)
}

func GetTestTokenWithClaims(principal string, roles []string, claims map[string]interface{}) *jwt.Token {
	realmAccessClaim := make(map[string]interface{}, 1)
	rolesClaim := make([]interface{}, len(roles))
	for i, role := range roles {
		rolesClaim[i] = role
	}
	realmAccessClaim["roles"] = rolesClaim
	mapClaims := jwt.MapClaims{}

	mapClaims["jti"] = uuid.NewString()
	mapClaims["iss"] = issuer()
	mapClaims["preferred_username"] = principal
	mapClaims["realm_access"] = realmAccessClaim
	for claimName, claimValue := range claims {
		mapClaims[claimName] = claimValue
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, mapClaims)
	token.Header["kid"] = mainKeyId
	return token
}

func GetSignedToken(principal string, roles []string) string {
	signedToken, _ := GetTestToken(principal, roles).SignedString(keys[mainKeyId])
	return signedToken
}

func GetSignedTokenWithClaims(principal string, roles []string, claims map[string]interface{}) string {
	signedToken, _ := GetTestTokenWithClaims(principal, roles, claims).SignedString(keys[mainKeyId])
	return signedToken
}

func GetMainSigningKey() rsa.PrivateKey {
	return *keys[mainKeyId]
}

func AddMockCertsEndpointHandler() {
	wellknownEndpoint := fmt.Sprintf(wellknownEndpointFormat, realm)
	certsEndpoint := fmt.Sprintf(certsEndpointFormat, realm)
	AddHandler(Contains(wellknownEndpoint),
		func(w http.ResponseWriter, r *http.Request) {
			oidcResponse := OidcResponse{
				JwksUri: fmt.Sprintf("%s/%s", mockIdpHost, certsEndpoint),
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			marshalledResponse, _ := json.Marshal(oidcResponse)
			w.Write(marshalledResponse)
		})
	AddHandler(Contains(certsEndpoint),
		func(w http.ResponseWriter, r *http.Request) {
			var certResponseKeys []CertResponseKey
			for kid, privateKey := range keys {
				certResponseKeys = append(certResponseKeys, createCertResponseKey(kid, privateKey))
			}
			certsResponse := CertResponse{Keys: certResponseKeys}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			marshalledResponse, _ := json.Marshal(certsResponse)
			w.Write(marshalledResponse)
		})
}

func createCertResponseKey(kid string, key *rsa.PrivateKey) CertResponseKey {
	publicKey := key.PublicKey
	eBuffer := new(bytes.Buffer)
	var eUint = uint64(publicKey.E)
	binary.Write(eBuffer, binary.BigEndian, &eUint)
	certResponseKey := CertResponseKey{
		Kid: kid,
		Kty: "RSA",
		Alg: "RS256",
		Use: "sig",
		N:   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(eBuffer.Bytes()),
	}
	return certResponseKey
}

func AuthHeaderValue(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}
