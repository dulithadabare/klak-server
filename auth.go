package main

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"

	jwt "github.com/golang-jwt/jwt"
)

// JwtAuth ...
type JwtAuth struct {
	jwk               *JWK
	jwkURL            string
	cognitoRegion     string
	cognitoUserPoolID string
}

// Config ...
type Config struct {
	CognitoRegion     string
	CognitoUserPoolID string
}

type KeySet struct {
	Alg string `json:"alg"`
	E   string `json:"e"`
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
}

// JWK ...
type JWK struct {
	Keys []KeySet `json:"keys"`
}

// MapKeys indexes each KeySet against its KID
func (jwk *JWK) MapKeys() map[string]KeySet {
	keymap := make(map[string]KeySet)
	for _, keys := range jwk.Keys {
		keymap[keys.Kid] = keys
	}
	return keymap
}

// NewAuth ...
func NewAuth(config *Config) *JwtAuth {
	a := &JwtAuth{
		cognitoRegion:     config.CognitoRegion,
		cognitoUserPoolID: config.CognitoUserPoolID,
	}

	a.jwkURL = fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s/.well-known/jwks.json", a.cognitoRegion, a.cognitoUserPoolID)
	err := a.CacheJWK()
	if err != nil {
		log.Fatal(err)
	}

	return a
}

// CacheJWK ...
func (a *JwtAuth) CacheJWK() error {
	req, err := http.NewRequest("GET", a.jwkURL, nil)
	if err != nil {
		return err
	}

	req.Header.Add("Accept", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	jwk := new(JWK)
	err = json.Unmarshal(body, jwk)
	if err != nil {
		return err
	}

	a.jwk = jwk
	return nil
}

// ParseJWT ...
func (a *JwtAuth) ParseJWT(tokenString string) (*jwt.Token, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.StandardClaims{}, func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("getting kid; not a string")
		}
		keymap := a.jwk.MapKeys()
		keyset, ok := keymap[kid]
		if !ok {
			return nil, fmt.Errorf("keyset not found for kid %s", kid)
		}
		key := convertKey(keyset.E, keyset.N)
		return key, nil
	})
	if err != nil {
		return token, fmt.Errorf("parsing jwt; %w", err)
	}

	return token, nil
}

//sub
func (a *JwtAuth) sub(token *jwt.Token) (string, error) {
	claims, ok := token.Claims.(*jwt.StandardClaims)
	if !ok {
		return "", errors.New("there is problem to get claims")
	}

	return claims.Subject, nil
}

// JWK ...
func (a *JwtAuth) JWK() *JWK {
	return a.jwk
}

// JWKURL ...
func (a *JwtAuth) JWKURL() string {
	return a.jwkURL
}

// https://gist.github.com/MathieuMailhos/361f24316d2de29e8d41e808e0071b13
func convertKey(rawE, rawN string) *rsa.PublicKey {
	decodedE, err := base64.RawURLEncoding.DecodeString(rawE)
	if err != nil {
		panic(err)
	}
	if len(decodedE) < 4 {
		ndata := make([]byte, 4)
		copy(ndata[4-len(decodedE):], decodedE)
		decodedE = ndata
	}
	pubKey := &rsa.PublicKey{
		N: &big.Int{},
		E: int(binary.BigEndian.Uint32(decodedE[:])),
	}
	decodedN, err := base64.RawURLEncoding.DecodeString(rawN)
	if err != nil {
		panic(err)
	}
	pubKey.N.SetBytes(decodedN)
	return pubKey
}
