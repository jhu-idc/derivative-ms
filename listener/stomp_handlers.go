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

func (h *JWTHandler) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	mId := m.Header.Get("message-id")

	var publicKey = []byte(env.GetOrPanic(VarDrupalJwtPublicKey))
	var privateKey = []byte(env.GetOrPanic(VarDrupalJwtPrivateKey))

	var (
		rawToken []byte
		token    *jwt.Token
		verifier jwt.Verifier
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
			return ctx, fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s", blockType)
		}
		if pubKey, err := x509.ParsePKIXPublicKey(block.Bytes); err != nil {
			return ctx, fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s: %w", blockType, err)
		} else {
			if rsaKey, ok := pubKey.(*rsa.PublicKey); ok {
				verifier, err = jwt.NewVerifierRS(token.Header().Algorithm, rsaKey)
			} else {
				return ctx, fmt.Errorf("handler: unable to verify JWT, need %T but got %T", &rsa.PublicKey{}, pubKey)
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
			return ctx, fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s", blockType)
		}
		if key, err := x509.ParseECPrivateKey(block.Bytes); err != nil {
			return ctx, fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s: %w", blockType, err)
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
			return ctx, fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s", blockType)
		}
		if pubKey, err := x509.ParsePKCS1PublicKey(block.Bytes); err != nil {
			return ctx, fmt.Errorf("handler: unable to verify JWT, invalid PEM encoded %s: %w", blockType, err)
		} else {
			verifier, err = jwt.NewVerifierPS(token.Header().Algorithm, pubKey)
		}

	default:
		return ctx, fmt.Errorf("handler: unknown or unsupported JWT algorithm '%s' for message-id %s",
			token.Header().Algorithm, mId)
	}

	if err != nil {
		return ctx, fmt.Errorf("handler: unable instantiate JWT Verifier for message-id %s: %w", mId, err)
	}

	err = verifier.Verify(token)

	if err != nil {
		return ctx, fmt.Errorf("handler: unable to verify JWT for message-id %s: %w",
			mId, err)
	}

	// check expiration
	expClaim := struct {
		Exp int64
	}{}
	token.DecodeClaims(&expClaim)
	if time.Now().After(time.Unix(expClaim.Exp, 0)) {
		// JWT is expired
		return ctx, fmt.Errorf("handler: JWT for message-id %s is expired on %s", mId, time.Unix(expClaim.Exp, 0).Format(time.RFC3339))
	}

	ctx = context.WithValue(ctx, MsgJwt, token)

	log.Printf("handler: verified JWT for message %s", mId)
	return ctx, nil
}

func (h *JWTHandler) Configure() error {
	var (
		jwtConfig *map[string]interface{}
		err       error
	)

	if jwtConfig, err = h.UnmarshalConfig(); err != nil {
		return fmt.Errorf("handler: unable to configure JWTHandler: %w", err)
	}

	if requireTokens, err := config.BoolValue(jwtConfig, "requireTokens"); err != nil {
		return fmt.Errorf("listener: unable to configure JWTHandler '%s': %w", h.Key, err)
	} else {
		h.RejectIfTokenMissing = requireTokens
	}

	if verifyTokens, err := config.BoolValue(jwtConfig, "verifyTokens"); err != nil {
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