package listener

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"derivative-ms/config"
	"derivative-ms/env"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/cristalhq/jwt/v4"
	"github.com/go-stomp/stomp/v3"
	"io"
	"log"
	"strings"
	"time"
)

const (
	VarDrupalJwtPublicKey  = "DRUPAL_JWT_PUBLIC_KEY"
	VarDrupalJwtPrivateKey = "DRUPAL_JWT_PRIVATE_KEY"
)

type CompositeStompHandler struct {
	Handlers []StompHandler
}

func (h CompositeStompHandler) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	var err error

	for _, handler := range h.Handlers {
		if ctx, err = handler.Handle(ctx, m); err != nil {
			return ctx, err
		}
	}

	return ctx, err
}

type JWTHandler struct {
	config.Configuration
	RejectIfTokenMissing bool `json:"requireTokens"`
	VerifyTokens         bool `json:"verifyTokens"`
}

type JWTLoggingHandler struct {
	config.Configuration
}

type StompLoggerHandler struct {
	Writer io.Writer
}

type MessageBody struct {
	// TODO add any other headers like destination or message id?
	Attachment struct {
		Content struct {
			Args           string `json:"args,omitempty"`
			DestinationUri string `json:"destination_uri"`
			UploadUri      string `json:"file_upload_uri"`
			MimeType       string `json:"mimetype"`
			SourceUri      string `json:"source_uri"`
		}
	}
}

func (h MessageBody) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	instance := &MessageBody{}
	if err := json.Unmarshal(m.Body, instance); err != nil {
		return ctx, err
	}

	return context.WithValue(context.WithValue(ctx, MsgBody, instance), MsgDestination, m.Destination), nil
}

func (h MessageBody) Configure(config.Configuration) error {
	// no-op
	return nil
}

func (h *JWTLoggingHandler) Configure(c config.Configuration) error {
	// no-op
	return nil
}

func (h *JWTLoggingHandler) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	var (
		rawToken []byte
		token    *jwt.Token
		err      error
	)

	mid := m.Header.Get("message-id")

	if authHeader := m.Header.Get("Authorization"); authHeader == "" {
		// token not found on the message
		log.Printf("handler: no JWT found on the message with id '%s'", mid)
		return ctx, nil
	} else {
		if strings.HasPrefix(authHeader, "Bearer ") {
			rawToken = []byte(authHeader[len("Bearer "):])
		} else {
			rawToken = []byte(authHeader)
		}
	}

	if token, err = jwt.ParseNoVerify(rawToken); err != nil {
		// token not parsable
		log.Printf("handler: JWT token could not be parsed from message: %s", err)
		return ctx, nil
	}

	err = verify(token, []byte(env.GetOrDefault(VarDrupalJwtPrivateKey, "")), []byte(env.GetOrDefault(VarDrupalJwtPublicKey, "")))

	if err != nil {
		log.Printf("handler: JWT could not be verified for message-id '%s': %s", mid, err)
	} else {
		log.Printf("handler: JWT verified for message-id '%s'", mid)
	}

	// Decode registered claims
	rClaims := jwt.RegisteredClaims{}

	if err := token.DecodeClaims(&jwt.RegisteredClaims{}); err != nil {
		log.Printf("handler: error decoding JWT claims for message-id '%s': %s", mid, err)
	} else if !rClaims.IsValidExpiresAt(time.Now()) {
		log.Printf("handler: JWT for message-id '%s' is expired on %s", mid, rClaims.ExpiresAt.Format(time.RFC3339))
	}

	// Decode all claims and log them
	claims := make(map[string]interface{})

	b := strings.Builder{}
	b.WriteString(fmt.Sprintf("JWT claims for message-id '%s'\n", mid))
	for k, v := range claims {
		b.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
	}

	log.Printf("handler: %s", b.String())

	return ctx, nil
}

func (h *JWTHandler) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	mId := m.Header.Get("message-id")

	// FIXME: we don't need a public key for RS256, and we may not need a "private" key for other algorithms.
	//  Figure out appropriate variable names, but we shouldn't panic until we know what keys are needed from the
	//  environment
	var publicKey = []byte(env.GetOrDefault(VarDrupalJwtPublicKey, ""))
	var privateKey = []byte(env.GetOrDefault(VarDrupalJwtPrivateKey, ""))

	var (
		rawToken []byte
		token    *jwt.Token
		err      error
	)

	if authHeader := m.Header.Get("Authorization"); authHeader == "" {
		if h.RejectIfTokenMissing {
			return ctx, errors.New("handler: message missing required JWT token")
		}
		// token not found on the message
		return ctx, nil
	} else {
		if strings.HasPrefix(authHeader, "Bearer ") {
			rawToken = []byte(authHeader[len("Bearer "):])
		} else {
			rawToken = []byte(authHeader)
		}
	}

	if token, err = jwt.ParseNoVerify(rawToken); err != nil {
		return ctx, err
	}

	err = verify(token, privateKey, publicKey)

	if err != nil {
		return ctx, fmt.Errorf("handler: unable to verify JWT for message-id %s: %w",
			mId, err)
	}

	// Decode registered claims and check expiration
	rClaims := jwt.RegisteredClaims{}

	if err := token.DecodeClaims(&jwt.RegisteredClaims{}); err != nil {
		return ctx, fmt.Errorf("handler: error decoding JWT claims for message-id '%s': %w", mId, err)
	} else if !rClaims.IsValidExpiresAt(time.Now()) {
		return ctx, fmt.Errorf("handler: JWT for message-id %s is expired on %s", mId, rClaims.ExpiresAt.Format(time.RFC3339))
	}

	ctx = context.WithValue(ctx, MsgJwt, token)

	log.Printf("handler: verified JWT for message %s", mId)
	return ctx, nil
}

func verify(token *jwt.Token, privateKey, publicKey []byte) error {

	var (
		verifier jwt.Verifier
		err      error
	)

	switch token.Header().Algorithm {
	case jwt.EdDSA:
		verifier, err = jwt.NewVerifierEdDSA(publicKey)

	case jwt.HS256:
		fallthrough
	case jwt.HS384:
		fallthrough
	case jwt.HS512:
		// TODO
		verifier, err = jwt.NewVerifierHS(token.Header().Algorithm, privateKey)

	case jwt.RS256:
		fallthrough
	case jwt.RS384:
		fallthrough
	case jwt.RS512:
		block, _ := pem.Decode(publicKey)
		blockType := "RSA PUBLIC KEY"
		if block == nil || block.Type != blockType {
			return fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s", blockType)
		}
		if pubKey, err := x509.ParsePKIXPublicKey(block.Bytes); err != nil {
			return fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s: %w", blockType, err)
		} else {
			if rsaKey, ok := pubKey.(*rsa.PublicKey); ok {
				verifier, err = jwt.NewVerifierRS(token.Header().Algorithm, rsaKey)
			} else {
				return fmt.Errorf("handler: unable to verify JWT, need %T but got %T", &rsa.PublicKey{}, pubKey)
			}
		}

	case jwt.ES256:
		fallthrough
	case jwt.ES384:
		fallthrough
	case jwt.ES512:
		block, _ := pem.Decode(privateKey)
		blockType := "EC PRIVATE KEY"
		if block == nil || block.Type != blockType {
			return fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s", blockType)
		}
		if key, err := x509.ParseECPrivateKey(block.Bytes); err != nil {
			return fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s: %w", blockType, err)
		} else {
			verifier, err = jwt.NewVerifierES(token.Header().Algorithm, &key.PublicKey)
		}

	case jwt.PS256:
		fallthrough
	case jwt.PS384:
		fallthrough
	case jwt.PS512:
		block, _ := pem.Decode(publicKey)
		blockType := "RSA PUBLIC KEY"
		if block == nil || block.Type != blockType {
			return fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s", blockType)
		}
		if pubKey, err := x509.ParsePKCS1PublicKey(block.Bytes); err != nil {
			return fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s: %w", blockType, err)
		} else {
			verifier, err = jwt.NewVerifierPS(token.Header().Algorithm, pubKey)
		}

	default:
		return fmt.Errorf("handler: unknown or unsupported JWT algorithm '%s'", token.Header().Algorithm)
	}

	if err != nil {
		return fmt.Errorf("handler: unable instantiate JWT Verifier: %w", err)
	}

	return verifier.Verify(token)
}

func (h *JWTHandler) Configure(c config.Configuration) error {
	var (
		jwtConfig *map[string]interface{}
		err       error
	)
	h.Configuration = c

	if jwtConfig, err = h.UnmarshalHandlerConfig(); err != nil {
		return fmt.Errorf("handler: unable to configure JWTHandler: %w", err)
	}

	if requireTokens, err := c.BoolValue(jwtConfig, "requireTokens"); err != nil {
		return fmt.Errorf("listener: unable to configure JWTHandler '%s': %w", h.Key, err)
	} else {
		h.RejectIfTokenMissing = requireTokens
	}

	if verifyTokens, err := c.BoolValue(jwtConfig, "verifyTokens"); err != nil {
		return fmt.Errorf("listener: unable to configure JWTHandler '%s': %w", h.Key, err)
	} else {
		h.VerifyTokens = verifyTokens
	}

	return nil
}

func (h StompLoggerHandler) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	var err error
	var prettyB []byte
	b := map[string]interface{}{}

	if err = json.Unmarshal(m.Body, &b); err != nil {
		return ctx, err
	}
	if prettyB, err = json.MarshalIndent(b, "", "  "); err != nil {
		return ctx, err
	}

	headers := strings.Builder{}
	for i := 0; i < m.Header.Len(); i++ {
		k, v := m.Header.GetAt(i)
		headers.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
	}

	fmt.Fprintf(h.Writer,
		"Content Type: %s\n"+
			"Destination: %s\n"+
			"Subscription: %s\n"+
			"Headers:\n%s"+
			"Body:\n"+
			"%+v\n",
		m.ContentType,
		m.Destination,
		m.Subscription.Id(),
		headers.String(),
		string(prettyB))

	return context.WithValue(ctx, msgFullBody, &b), nil
}

func (h StompLoggerHandler) Configure(config.Configuration) error {
	// no-op
	return nil
}
