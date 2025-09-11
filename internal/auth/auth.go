package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type TokenType string

const (
	TokenTypeAccess TokenType = "tubely-access"
)

var ErrNoAuthHeaderIncluded = errors.New("no auth header included in request")

func HashPassword(password string) (string, error) {
	dat, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(dat), nil
}

func CheckPasswordHash(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func MakeJWT(
	userID uuid.UUID,
	tokenSecret string,
	expiresIn time.Duration,
) (string, error) {
	signingKey := []byte(tokenSecret)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    string(TokenTypeAccess),
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   userID.String(),
	})
	return token.SignedString(signingKey)
}

// This function signature tells us it takes two string parameters:
//	* tokenString: The actual JWT token (a long encoded string)
//	* tokenSecret: The secret key used to verify the token's authenticity
// And it returns:
// 	* uuid.UUID: The user's ID if validation succeeds
// 	* error: Any error that occurred during validation
func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	// create an empty struct to hold the "claims" (the data) from inside the JWT token. RegisteredClaims 
	// is a standard struct that contains common JWT fields like:
	//	* Subject (usually the user ID)
	// 	* ExpiresAt (when the token expires)
	// 	* IssuedAt (when the token was created)
	claimsStruct := jwt.RegisteredClaims{}
	// ParseWithClaims does three things:
	//	1. Decodes the JWT token string back into its parts
	//	2. Verifies the signature using the secret key (that anonymous function returns the secret as bytes)
	//	3. Populates the claimsStruct with the decoded data
	token, err := jwt.ParseWithClaims(
		tokenString,
		&claimsStruct,
		// The anonymous function is a callback that provides the secret key for verification:
		// Takes a JWT token as input
		// Returns an interface{} (which can be any type) and an error
		// []byte(tokenSecret): Converts the tokenSecret string into a byte slice
		func(token *jwt.Token) (interface{}, error) { return []byte(tokenSecret), nil },
	)
	if err != nil {
		// if there's an error, return an empty/zero uuid and the error:
		return uuid.Nil, err
	}

	userIDString, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, err
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		return uuid.Nil, err
	}
	if issuer != string(TokenTypeAccess) {
		return uuid.Nil, errors.New("invalid issuer")
	}

	id, err := uuid.Parse(userIDString)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid user ID: %w", err)
	}
	return id, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	// Get the Authorization header value:
	authHeader := headers.Get("Authorization")
	// if 'authHeader' is empty, return an error:
	if authHeader == "" {
		return "", ErrNoAuthHeaderIncluded
	}
	// Split the header by spaces: ["Bearer", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."]:
	splitAuth := strings.Split(authHeader, " ")
	// Validate it starts with "Bearer" and has at least 2 parts:
	if len(splitAuth) < 2 || splitAuth[0] != "Bearer" {
		return "", errors.New("malformed authorization header")
	}
	// Return just the token part (everything after "Bearer "):
	return splitAuth[1], nil
}

func MakeRefreshToken() (string, error) {
	token := make([]byte, 32)
	_, err := rand.Read(token)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(token), nil
}

func GetAPIKey(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")
	if authHeader == "" {
		return "", ErrNoAuthHeaderIncluded
	}
	splitAuth := strings.Split(authHeader, " ")
	if len(splitAuth) < 2 || splitAuth[0] != "ApiKey" {
		return "", errors.New("malformed authorization header")
	}

	return splitAuth[1], nil
}
