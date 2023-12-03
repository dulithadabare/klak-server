package main

import (
	"errors"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
)

func respondWithError(c *gin.Context, code int, message interface{}) {
	c.AbortWithStatusJSON(code, gin.H{"error": message})
}

var (
	// ErrEmptyAuthHeader can be thrown if authing with a HTTP header, the Auth header needs to be set
	ErrEmptyAuthHeader = errors.New("auth header is empty")

	// ErrInvalidAuthHeader indicates auth header is invalid, could for example have the wrong Realm name
	ErrInvalidAuthHeader = errors.New("auth header is invalid")
)

func jwtFromHeader(c *gin.Context) (string, error) {
	authHeader := c.Request.Header.Get("Authorization")

	if authHeader == "" {
		return "", ErrEmptyAuthHeader
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if !(len(parts) == 2 && parts[0] == "Bearer") {
		return "", ErrInvalidAuthHeader
	}

	return parts[1], nil
}

func CognitoJWTTokenAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		idToken, err := jwtFromHeader(c)

		if err != nil {
			log.Printf("Error: %v\n", err.Error())
			respondWithError(c, 401, err.Error())
			return
		}

		token, err := cognitoJWTAuth.ParseJWT(idToken)
		if err != nil {
			log.Printf("Error: %v\n", err.Error())
			respondWithError(c, 401, "Invalid API token: Parse failed")
			return
		}

		if !token.Valid {
			log.Printf("Error: %v\n", err.Error())
			respondWithError(c, 401, "Invalid API token")
			return
		}

		// token, err := authClient.VerifyIDToken(c, idToken)
		// if err != nil {
		// 	log.Printf("Error: %v\n", err.Error())
		// 	respondWithError(c, 401, "Invalid API token")
		// 	return
		// }

		// log.Printf("Verified ID token: %v\n", token)
		// log.Printf("Verified ID token: %s %s\n", token.UID, token.Claims["phone_number"])

		// Set example variable
		sub, err := cognitoJWTAuth.sub(token)

		if err != nil {
			log.Printf("Error: %v\n", err.Error())
			respondWithError(c, 401, "Sub does not exist")
			return
		}

		c.Set(uidKey, sub)

		c.Next()
	}
}

func FirebaseJWTTokenAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		idToken, err := jwtFromHeader(c)

		if err != nil {
			log.Printf("Error: %v\n", err.Error())
			respondWithError(c, 401, err.Error())
			return
		}

		token, err := cognitoJWTAuth.ParseJWT(idToken)
		if err != nil {
			log.Printf("Error: %v\n", err.Error())
			respondWithError(c, 401, "Invalid API token")
			return
		}

		if !token.Valid {
			log.Printf("Error: %v\n", err.Error())
			respondWithError(c, 401, "Invalid API token")
			return
		}

		// token, err := authClient.VerifyIDToken(c, idToken)
		// if err != nil {
		// 	log.Printf("Error: %v\n", err.Error())
		// 	respondWithError(c, 401, "Invalid API token")
		// 	return
		// }

		// log.Printf("Verified ID token: %v\n", token)
		// log.Printf("Verified ID token: %s %s\n", token.UID, token.Claims["phone_number"])

		// Set example variable
		sub, err := cognitoJWTAuth.sub(token)

		if err != nil {
			log.Printf("Error: %v\n", err.Error())
			respondWithError(c, 401, "Sub does not")
			return
		}

		c.Set(uidKey, sub)

		c.Next()
	}
}
