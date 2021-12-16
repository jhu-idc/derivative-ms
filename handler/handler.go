package handler

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"derivative-ms/api"
	"derivative-ms/cmd"
	"derivative-ms/config"
	"derivative-ms/drupal"
	"derivative-ms/drupal/request"
	"derivative-ms/env"
	"encoding/pem"
	"fmt"
	"github.com/cristalhq/jwt/v4"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

var httpClient = &http.Client{}

type CompositeHandler struct {
	Handlers []api.Handler
}

// TODO use plugin mechanism for handlers?
type ImageMagickHandler struct {
	config.Configuration
	Drupal           drupal.Client
	CommandBuilder   cmd.Builder
	DefaultMediaType string
	AcceptedFormats  map[string]struct{}
	CommandPath      string
}

type TesseractHandler struct {
	config.Configuration
	Drupal         drupal.Client
	CommandBuilder cmd.Builder
	CommandPath    string
}

type Pdf2TextHandler struct {
	config.Configuration
	Drupal          drupal.Client
	CommandBuilder  cmd.Builder
	CommandPath     string
	AcceptedFormats map[string]struct{}
}

type FFMpegHandler struct {
	config.Configuration
	Drupal             drupal.Client
	CommandBuilder     cmd.Builder
	DefaultMediaType   string
	AcceptedFormatsMap map[string]string
	CommandPath        string
}

func (h *TesseractHandler) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	if ctx.Value(api.MsgDestination).(string) != config.HypercubeDestination {
		return ctx, nil
	}

	var (
		// original image from Drupal
		sourceStream io.ReadCloser
		// tesseract stdin
		tStdin io.WriteCloser
		// tesseract stdout
		tStdout io.ReadCloser

		err error

		logger = newLogger("TesseractHandler", ctx.Value(api.MsgId))

		reqCtx = request.New().WithToken(t)

		cmd *exec.Cmd
	)

	cmd, err = h.CommandBuilder.Build(h.CommandPath, t, b)
	if err != nil {
		return ctx, err
	}

	// Buffer the source stream's first 512 bytes and sniff the content
	sourceStream, err = h.Drupal.Get(*reqCtx, b.Attachment.Content.SourceUri)
	bufSource := bufio.NewReaderSize(sourceStream, 512)

	if sniff, err := bufSource.Peek(512); err != nil {
		return ctx, err
	} else {
		contentType := http.DetectContentType(sniff)
		logger.Printf("handler: sniffed media type: %s", contentType)
		if strings.HasPrefix(contentType, "application/pdf") {
			// then pdf2text handler should handle this
			return ctx, nil
		}
	}

	defer sourceStream.Close()
	if err != nil {
		return ctx, err
	}

	// open tesseract stdin and stdout
	if tStdin, err = cmd.StdinPipe(); err != nil {
		return ctx, err
	}
	if tStdout, err = cmd.StdoutPipe(); err != nil {
		return ctx, err
	}

	stdErr, _ := cmd.StderrPipe()
	go func() {
		f, _ := ioutil.TempFile("/tmp", "tesseract-")
		logger.Printf("handler: ending debug output to '%s'", f.Name())
		io.Copy(f, stdErr)
	}()

	go func() {
		var ioErr error
		defer func() {
			tStdin.Close()
			if ioErr != nil {
				logger.Printf("handler: error copying stream from '%s' to stdin of '%s': %s",
					b.Attachment.Content.SourceUri, h.CommandPath, ioErr)
			}
		}()
		_, ioErr = io.Copy(tStdin, sourceStream)
	}()

	logger.Printf("handler: running '%s'", cmd.String())
	if err = cmd.Start(); err != nil {
		return ctx, err
	}

	reqCtx.WithHeader("Content-Type", "text/plain").
		WithHeader("Content-Location", b.Attachment.Content.UploadUri)
	_, err = h.Drupal.Put(*reqCtx, b.Attachment.Content.DestinationUri, tStdout)

	if err != nil {
		return ctx, err
	}

	return ctx, cmd.Wait()
}

func (h *TesseractHandler) Configure(c config.Configuration) error {
	var (
		handlerConfig *map[string]interface{}
		err           error
	)
	h.Configuration = c

	if handlerConfig, err = h.UnmarshalHandlerConfig(); err != nil {
		return fmt.Errorf("handler: unable to configure TesseractHandler: %w", err)
	}

	if h.CommandPath, err = config.StringValue(handlerConfig, "commandPath"); err != nil {
		return fmt.Errorf("handler: unable to configure TesseractHandler '%s', parameter '%s': %w", h.Key, "commandPath", err)
	}

	h.Drupal = drupal.HttpImpl{HttpClient: drupal.DefaultClient}

	h.CommandBuilder = &cmd.Tesseract{}

	return nil
}

func (h *Pdf2TextHandler) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	if ctx.Value(api.MsgDestination).(string) != config.HypercubeDestination {
		return ctx, nil
	}

	var (
		// original image from Drupal
		sourceStream io.ReadCloser
		// tesseract stdin
		tStdin io.WriteCloser
		// tesseract stdout
		tStdout io.ReadCloser

		err error

		logger = newLogger("Pdf2TextHandler", ctx.Value(api.MsgId))

		reqCtx = request.New().WithToken(t)

		cmd *exec.Cmd
	)

	cmd, err = h.CommandBuilder.Build(h.CommandPath, t, b)
	if err != nil {
		return ctx, err
	}

	// Buffer the source stream's first 512 bytes and sniff the content
	sourceStream, err = h.Drupal.Get(*reqCtx, b.Attachment.Content.SourceUri)
	bufSource := bufio.NewReaderSize(sourceStream, 512)

	if sniff, err := bufSource.Peek(512); err != nil {
		return ctx, err
	} else {
		contentType := http.DetectContentType(sniff)
		logger.Printf("handler: sniffed media type: %s", contentType)
		if !strings.HasPrefix(contentType, "application/pdf") {
			// then tesseract handler should handle this
			return ctx, nil
		}
	}

	if tStdin, err = cmd.StdinPipe(); err != nil {
		return ctx, err
	}

	if tStdout, err = cmd.StdoutPipe(); err != nil {
		return ctx, err
	}

	go func() {
		var ioErr error
		defer func() {
			tStdin.Close()
			if ioErr != nil {
				logger.Printf("handler: error copying stream from '%s' to stdin of '%s': %s",
					b.Attachment.Content.SourceUri, h.CommandPath, ioErr)
			}
		}()
		_, ioErr = io.Copy(tStdin, sourceStream)
	}()

	if err = cmd.Start(); err != nil {
		return ctx, err
	}

	reqCtx.WithHeader("Content-Type", "text/plain").
		WithHeader("Content-Location", b.Attachment.Content.UploadUri)
	_, err = h.Drupal.Put(*reqCtx, b.Attachment.Content.DestinationUri, tStdout)

	if err != nil {
		return ctx, err
	}

	return ctx, cmd.Wait()
}

func (h *Pdf2TextHandler) Configure(c config.Configuration) error {
	var (
		handlerConfig *map[string]interface{}
		err           error
	)
	h.Configuration = c

	if handlerConfig, err = h.UnmarshalHandlerConfig(); err != nil {
		return fmt.Errorf("handler: unable to configure Pdf2TextHandler: %w", err)
	}

	if h.CommandPath, err = config.StringValue(handlerConfig, "commandPath"); err != nil {
		return fmt.Errorf("handler: unable to configure Pdf2TextHandler '%s', parameter '%s': %w", h.Key, "commandPath", err)
	}

	h.Drupal = drupal.HttpImpl{HttpClient: drupal.DefaultClient}

	h.CommandBuilder = cmd.Pdf2Text{}

	return nil
}

func (h *ImageMagickHandler) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	if ctx.Value(api.MsgDestination).(string) != config.HoudiniDestination {
		return ctx, nil
	}

	var (
		mid    = ctx.Value(api.MsgId)
		logger = newLogger("ImageMagickHandler", mid)
		cmd    *exec.Cmd
		err    error
	)

	// Remove any tmp files left behind due to a crash or unclean shutdown of Imagemagick
	defer func() {
		entries, err := os.ReadDir(os.TempDir())
		if err != nil {
			logger.Printf("handler: unable to clean up Imagemagick files '%s'", err)
			return
		}

		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "magick-") {
				info, _ := entry.Info()
				logger.Printf("handler: cleaning up Imagemagick temporary file '%s': %v", entry.Name(), info)
			}
		}
	}()

	// Set a default mime type (parity with PHP controller)
	if b.Attachment.Content.MimeType == "" {
		b.Attachment.Content.MimeType = h.DefaultMediaType
	}

	// Map the requested IANA media type to a supported imagemagick output format
	if _, ok := h.AcceptedFormats[b.Attachment.Content.MimeType]; !ok {
		return ctx, fmt.Errorf("[%s] [%s] handler: convert does not support mime type '%s'", "ImageMagickHandler", mid, b.Attachment.Content.MimeType)
	}

	if cmd, err = h.CommandBuilder.Build(h.CommandPath, t, b); err != nil {
		return ctx, err
	}

	// GET the original image from Drupal
	// Stream the original image into Imagemagick
	// PUT the output of convert (i.e. the derivative) to Drupal
	var (
		// original image from Drupal
		sourceStream io.ReadCloser
		// imagemagick stdin
		imgStdin io.WriteCloser
		// imagemagick stdout
		imgStdout io.ReadCloser
		// imagemagick stderr
		imgStderr io.ReadCloser

		reqCtx = request.New().WithToken(t)
	)

	// GET original image from Drupal
	sourceStream, err = h.Drupal.Get(*reqCtx, b.Attachment.Content.SourceUri)
	defer sourceStream.Close()
	if err != nil {
		return ctx, err
	}

	// open imagemagick stdin, stdout, and stderr
	if imgStdin, err = cmd.StdinPipe(); err != nil {
		return ctx, err
	}
	if imgStdout, err = cmd.StdoutPipe(); err != nil {
		return ctx, err
	}
	if imgStderr, err = cmd.StderrPipe(); err != nil {
		return ctx, err
	}

	// copy the source image to imagemagick's stdin, and close stdin after
	go func() {
		var ioErr error
		defer func() {
			imgStdin.Close()
			if ioErr != nil {
				logger.Printf("handler: error copying stream from '%s' to stdin of '%s': %s",
					b.Attachment.Content.SourceUri, h.CommandPath, ioErr)
			}
		}()
		_, ioErr = io.Copy(imgStdin, sourceStream)
	}()

	// if there is an error when exiting, attempt to copy out stderr, otherwise close it
	defer func() {
		if err != nil {
			b := &bytes.Buffer{}
			if _, err := io.Copy(b, imgStderr); err != nil {
				logger.Printf("handler: there was an error executing ImageMagick, but the stderr could not be captured: '%s'", err)
			} else {
				logger.Printf("handler: there was an error executing ImageMagick, stderr follows:\n%s", b)
			}
		}

		imgStderr.Close()
	}()

	// start imagemagick convert
	logger.Printf("handler: executing %+v\n", cmd)
	if err := cmd.Start(); err != nil {
		return ctx, err
	}

	// PUT the derivative to Drupal, using stdout from imagemagick
	reqCtx.WithHeader("Content-Location", b.Attachment.Content.UploadUri).
		WithHeader("Content-Type", b.Attachment.Content.MimeType)

	_, err = h.Drupal.Put(*reqCtx, b.Attachment.Content.DestinationUri, imgStdout)

	if err != nil {
		return ctx, err
	}

	// wait for imagemagick convert to finish
	return ctx, cmd.Wait()
}

func (h *ImageMagickHandler) Configure(c config.Configuration) error {
	return h.configure(c, false)
}

func (h *ImageMagickHandler) configure(c config.Configuration, ignoreErr bool) error {
	var (
		convertConfig *map[string]interface{}
		formats       []string
		err           error
	)
	h.Configuration = c

	if convertConfig, err = h.UnmarshalHandlerConfig(); err != nil && !ignoreErr {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler: %w", err)
	}

	if h.CommandPath, err = config.StringValue(convertConfig, "commandPath"); err != nil && !ignoreErr {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler '%s', parameter '%s': %w", h.Key, "commandPath", err)
	}

	if h.DefaultMediaType, err = config.StringValue(convertConfig, "defaultMediaType"); err != nil && !ignoreErr {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler '%s', parameter '%s': %w", h.Key, "defaultMediaType", err)
	}

	if formats, err = config.SliceStringValue(convertConfig, "acceptedFormats"); err != nil && !ignoreErr {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler '%s', parameter '%s': %w", h.Key, "acceptedFormats", err)
	}

	h.AcceptedFormats = make(map[string]struct{})

	for _, f := range formats {
		h.AcceptedFormats[f] = struct{}{}
	}

	h.Drupal = drupal.HttpImpl{HttpClient: drupal.DefaultClient}

	h.CommandBuilder = cmd.ImageMagick{}

	return nil
}

func (h *FFMpegHandler) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	if ctx.Value(api.MsgDestination).(string) != config.HomarusDestination {
		return ctx, nil
	}

	logger := newLogger("FFMpegHandler", ctx.Value(api.MsgId))

	// Set a default mime type (parity with PHP controller)
	if b.Attachment.Content.MimeType == "" {
		b.Attachment.Content.MimeType = h.DefaultMediaType
	}

	// Compose a PUT request to Drupal, and stream the output of FFmpeg as the PUT request body

	// GET the original image from Drupal
	// Stream the original image into FFmpeg
	// PUT the output of convert (i.e. the derivative) to Drupal
	var (
		cmd          *exec.Cmd
		ffmpegStdout io.ReadCloser
		err          error
	)

	if cmd, err = h.CommandBuilder.Build(h.CommandPath, t, b); err != nil {
		return ctx, err
	}

	// open ffmpeg stdout
	if ffmpegStdout, err = cmd.StdoutPipe(); err != nil {
		return ctx, err
	}

	//stdErr, _ := cmd.StderrPipe()
	//go func() {
	//	f, _ := ioutil.TempFile("/tmp", "ffmpeg-")
	//	log.Printf("Sending debug output to '%s'", f.Name())
	//	io.Copy(f, stdErr)
	//}()

	// start ffmpeg
	logger.Printf("handler: executing %+v\n", cmd)
	if err := cmd.Start(); err != nil {
		return ctx, err
	}

	// PUT the derivative to Drupal, using stdout from ffmpeg
	reqCtx := request.New().WithToken(t).
		WithHeader("Content-Location", b.Attachment.Content.UploadUri).
		WithHeader("Content-Type", b.Attachment.Content.MimeType)
	_, err = h.Drupal.Put(*reqCtx, b.Attachment.Content.DestinationUri, ffmpegStdout)

	if err != nil {
		return ctx, err
	}

	// wait for ffmpeg to finish
	return ctx, cmd.Wait()
}

func (h *FFMpegHandler) Configure(c config.Configuration) error {
	var (
		ffmpegConfig *map[string]interface{}
		formats      map[string]interface{}
		err          error
	)
	h.Configuration = c

	if ffmpegConfig, err = h.UnmarshalHandlerConfig(); err != nil {
		return fmt.Errorf("handler: unable to configure FFMpegHandler: %w", err)
	}

	if h.CommandPath, err = config.StringValue(ffmpegConfig, "commandPath"); err != nil {
		return fmt.Errorf("handler: unable to configure FFMpegHandler '%s', parameter '%s': %w", h.Key, "commandPath", err)
	}

	if h.DefaultMediaType, err = config.StringValue(ffmpegConfig, "defaultMediaType"); err != nil {
		return fmt.Errorf("handler: unable to configure FFMpegHandler '%s', parameter '%s': %w", h.Key, "defaultMediaType", err)
	}

	if formats, err = config.MapValue(ffmpegConfig, "acceptedFormatsMap"); err != nil {
		return fmt.Errorf("handler: unable to configure FFMpegHandler '%s', parameter '%s': %w", h.Key, "acceptedFormatsMap", err)
	}

	h.AcceptedFormatsMap = make(map[string]string)

	for k, v := range formats {
		h.AcceptedFormatsMap[k] = v.(string)
	}

	h.Drupal = drupal.HttpImpl{HttpClient: drupal.DefaultClient}

	h.CommandBuilder = cmd.FFMpeg{AcceptedFormatsMap: h.AcceptedFormatsMap}

	return nil
}

func (h CompositeHandler) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	var err error

	for _, handler := range h.Handlers {
		if ctx, err = handler.Handle(ctx, t, b); err != nil {
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

func (h *JWTLoggingHandler) Configure(c config.Configuration) error {
	// no-op
	return nil
}

func (h *JWTLoggingHandler) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	// TODO: handle a nil token, congruent with handler configuration
	var err error
	privateKey := []byte(env.GetOrDefault(config.VarDrupalJwtPrivateKey, ""))
	publicKey := []byte(env.GetOrDefault(config.VarDrupalJwtPublicKey, ""))
	logger := newLogger("JWTLoggingHandler", ctx.Value(api.MsgId))

	err = verify(t, privateKey, publicKey)

	if err != nil {
		logger.Printf("handler: JWT could not be verified: %s", err)
	} else {
		logger.Printf("handler: JWT verified")
	}

	// Decode all claims and log them
	claims := make(map[string]interface{})
	if err := t.DecodeClaims(&claims); err != nil {
		logger.Printf("handler: error decoding JWT claims: %s", err)
	}

	expired := time.Time{}
	builder := strings.Builder{}
	builder.WriteString("JWT claims:\n")
	for k, v := range claims {
		switch k {
		case "exp":
			expTime := time.Unix(int64(v.(float64)), 0)
			if time.Now().After(expTime) {
				expired = expTime
			}
			builder.WriteString(fmt.Sprintf("  %s: %v (%s)\n", k, v, expTime.Format(time.RFC3339)))
		case "iat":
			builder.WriteString(fmt.Sprintf("  %s: %v (%s)\n", k, v, time.Unix(int64(v.(float64)), 0).Format(time.RFC3339)))
		default:
			builder.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
	}

	logger.Printf("handler: %s", builder.String())
	if !expired.IsZero() {
		logger.Printf("handler: JWT expired at %s", expired.Format(time.RFC3339))
	}

	return ctx, nil
}

func (h *JWTHandler) Handle(ctx context.Context, t *jwt.Token, m *api.MessageBody) (context.Context, error) {
	// TODO: handle a nil token, congruent with handler configuration

	// FIXME: we don't need a public key for RS256, and we may not need a "private" key for other algorithms.
	//  Figure out appropriate variable names, but we shouldn't panic until we know what keys are needed from the
	//  environment
	var publicKey = []byte(env.GetOrDefault(config.VarDrupalJwtPublicKey, ""))
	var privateKey = []byte(env.GetOrDefault(config.VarDrupalJwtPrivateKey, ""))
	var err error
	logger := newLogger("JWTHandler", ctx.Value(api.MsgId))

	err = verify(t, privateKey, publicKey)

	if err != nil {
		return ctx, fmt.Errorf("handler: unable to verify JWT for message-id %s: %w",
			ctx.Value(api.MsgId), err)
	}

	// Decode registered claims and check expiration
	rClaims := jwt.RegisteredClaims{}

	if err := t.DecodeClaims(&jwt.RegisteredClaims{}); err != nil {
		return ctx, fmt.Errorf("handler: error decoding JWT claims for message-id '%s': %w", ctx.Value(api.MsgId), err)
	} else if !rClaims.IsValidExpiresAt(time.Now()) {
		return ctx, fmt.Errorf("handler: JWT for message-id %s is expired on %s", ctx.Value(api.MsgId), rClaims.ExpiresAt.Format(time.RFC3339))
	}

	ctx = context.WithValue(ctx, api.MsgJwt, t)

	logger.Printf("handle: verified JWT for message %s", ctx.Value(api.MsgId))
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
		return fmt.Errorf("handle: unable to configure JWTHandler: %w", err)
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

func newLogger(handlerName string, messageId interface{}) *log.Logger {
	return newLoggerWithPrefix(fmt.Sprintf("[%s] [%s] ", handlerName, messageId))
}

func newLoggerWithPrefix(prefix string) *log.Logger {
	return log.New(log.Writer(), prefix, log.Flags())
}

func asBearer(token *jwt.Token) string {
	return fmt.Sprintf("Bearer %s", token)
}
