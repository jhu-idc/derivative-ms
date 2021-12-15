package handler

import (
	"bufio"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"derivative-ms/api"
	"derivative-ms/config"
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

type CompositeHandler struct {
	Handlers []api.Handler
}

// TODO use plugin mechanism for handlers?
type ImageMagickHandler struct {
	config.Configuration
	DefaultMediaType string
	AcceptedFormats  map[string]struct{}
	CommandPath      string
}

type TesseractHandler struct {
	config.Configuration
	CommandPath string
}

type Pdf2TextHandler struct {
	config.Configuration
	CommandPath     string
	AcceptedFormats map[string]struct{}
}

type FFMpegHandler struct {
	config.Configuration
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
	)

	var cmdArgs []string
	cmdArgs = append(cmdArgs, h.CommandPath)
	cmdArgs = append(cmdArgs, "stdin", "stdout")
	if trimmedArgs := strings.TrimSpace(b.Attachment.Content.Args); len(trimmedArgs) > 0 {
		for _, addlArg := range strings.Split(trimmedArgs, " ") {
			cmdArgs = append(cmdArgs, addlArg)
		}
	}
	cmd := &exec.Cmd{
		Path: h.CommandPath,
		Args: cmdArgs,
	}

	// Buffer the source stream's first 512 bytes and sniff the content
	sourceStream, err = getResourceStream(b.Attachment.Content.SourceUri, t, nil)
	bufSource := bufio.NewReaderSize(sourceStream, 512)

	if sniff, err := bufSource.Peek(512); err != nil {
		return ctx, err
	} else {
		contentType := http.DetectContentType(sniff)
		log.Printf("Sniffed media type: %s", contentType)
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
		log.Printf("Sending debug output to '%s'", f.Name())
		io.Copy(f, stdErr)
	}()

	go func() {
		var ioErr error
		defer func() {
			tStdin.Close()
			if ioErr != nil {
				log.Printf("hander: error copying stream from '%s' to stdin of '%s': %s",
					b.Attachment.Content.SourceUri, h.CommandPath, ioErr)
			}
		}()
		_, ioErr = io.Copy(tStdin, sourceStream)
	}()

	log.Printf("Running '%s'", cmd.String())
	if err = cmd.Start(); err != nil {
		return ctx, err
	}

	_, err = putResourceStream(b.Attachment.Content.DestinationUri, t, tStdout, map[string]string{
		"Content-Type":     "text/plain",
		"Content-Location": b.Attachment.Content.UploadUri,
	})

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
	)

	var cmdArgs []string
	cmdArgs = append(cmdArgs, h.CommandPath)
	if trimmedArgs := strings.TrimSpace(b.Attachment.Content.Args); len(trimmedArgs) > 0 {
		for _, addlArg := range strings.Split(trimmedArgs, " ") {
			cmdArgs = append(cmdArgs, addlArg)
		}
	}
	cmdArgs = append(cmdArgs, "-", "-")
	cmd := &exec.Cmd{
		Path: h.CommandPath,
		Args: cmdArgs,
	}

	// Buffer the source stream's first 512 bytes and sniff the content
	sourceStream, err = getResourceStream(b.Attachment.Content.SourceUri, t, nil)
	bufSource := bufio.NewReaderSize(sourceStream, 512)

	if sniff, err := bufSource.Peek(512); err != nil {
		return ctx, err
	} else {
		contentType := http.DetectContentType(sniff)
		log.Printf("Sniffed media type: %s", contentType)
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
				log.Printf("hander: error copying stream from '%s' to stdin of '%s': %s",
					b.Attachment.Content.SourceUri, h.CommandPath, ioErr)
			}
		}()
		_, ioErr = io.Copy(tStdin, sourceStream)
	}()

	if err = cmd.Start(); err != nil {
		return ctx, err
	}

	_, err = putResourceStream(b.Attachment.Content.DestinationUri, t, tStdout, map[string]string{
		"Content-Type":     "text/plain",
		"Content-Location": b.Attachment.Content.UploadUri,
	})

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

	return nil
}

func (h *ImageMagickHandler) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	if ctx.Value(api.MsgDestination).(string) != config.HoudiniDestination {
		return ctx, nil
	}

	// Remove any tmp files left behind due to a crash or unclean shutdown of Imagemagick
	defer func() {
		entries, err := os.ReadDir(os.TempDir())
		if err != nil {
			log.Printf("handle: unable to clean up Imagemagick files '%s'", err)
			return
		}

		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "magick-") {
				info, _ := entry.Info()
				log.Printf("handler: cleaning up Imagemagick temporary file '%s': %v", entry.Name(), info)
			}
		}
	}()

	// Set a default mime type (parity with PHP controller)
	if b.Attachment.Content.MimeType == "" {
		b.Attachment.Content.MimeType = h.DefaultMediaType
	}

	convertFormat := b.Attachment.Content.MimeType[strings.LastIndex(b.Attachment.Content.MimeType, "/")+1:]

	// Map the requested IANA media type to a supported imagemagick output format
	if _, ok := h.AcceptedFormats[b.Attachment.Content.MimeType]; !ok {
		return ctx, fmt.Errorf("handler: convert does not support mime type '%s'", b.Attachment.Content.MimeType)
	}

	// Manually compose the command arguments so that the additional args supplied with the message
	// are properly parsed
	var cmdArgs []string
	cmdArgs = append(cmdArgs, h.CommandPath)
	cmdArgs = append(cmdArgs, "-")
	// "-thumbnail 100x100", or ""
	if trimmedArgs := strings.TrimSpace(b.Attachment.Content.Args); len(trimmedArgs) > 0 {
		for _, addlArg := range strings.Split(trimmedArgs, " ") {
			cmdArgs = append(cmdArgs, addlArg)
		}
	}
	cmdArgs = append(cmdArgs, fmt.Sprintf("%s:-", convertFormat))
	cmd := &exec.Cmd{
		Path: h.CommandPath,
		Args: cmdArgs,
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

		err error
	)

	// GET original image from Drupal
	sourceStream, err = getResourceStream(b.Attachment.Content.SourceUri, t, nil)
	defer sourceStream.Close()
	if err != nil {
		return ctx, err
	}

	// open imagemagick stdin and stdout
	if imgStdin, err = cmd.StdinPipe(); err != nil {
		return ctx, err
	}
	if imgStdout, err = cmd.StdoutPipe(); err != nil {
		return ctx, err
	}

	// copy the source image to imagemagick's stdin, and close stdin after
	go func() {
		var ioErr error
		defer func() {
			imgStdin.Close()
			if ioErr != nil {
				log.Printf("hander: error copying stream from '%s' to stdin of '%s': %s",
					b.Attachment.Content.SourceUri, h.CommandPath, ioErr)
			}
		}()
		_, ioErr = io.Copy(imgStdin, sourceStream)
	}()

	// start imagemagick convert
	log.Printf("Executing %+v\n", cmd)
	if err := cmd.Start(); err != nil {
		return ctx, err
	}

	// PUT the derivative to Drupal, using stdout from imagemagick
	_, err = putResourceStream(b.Attachment.Content.DestinationUri, t, imgStdout, map[string]string{
		"Content-Location": b.Attachment.Content.UploadUri,
		"Content-Type":     b.Attachment.Content.MimeType,
	})

	if err != nil {
		return ctx, err
	}

	// wait for imagemagick convert to finish
	return ctx, cmd.Wait()
}

func (h *ImageMagickHandler) Configure(c config.Configuration) error {
	var (
		convertConfig *map[string]interface{}
		formats       []string
		err           error
	)
	h.Configuration = c

	if convertConfig, err = h.UnmarshalHandlerConfig(); err != nil {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler: %w", err)
	}

	if h.CommandPath, err = config.StringValue(convertConfig, "commandPath"); err != nil {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler '%s', parameter '%s': %w", h.Key, "commandPath", err)
	}

	if h.DefaultMediaType, err = config.StringValue(convertConfig, "defaultMediaType"); err != nil {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler '%s', parameter '%s': %w", h.Key, "defaultMediaType", err)
	}

	if formats, err = config.SliceStringValue(convertConfig, "acceptedFormats"); err != nil {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler '%s', parameter '%s': %w", h.Key, "acceptedFormats", err)
	}

	h.AcceptedFormats = make(map[string]struct{})

	for _, f := range formats {
		h.AcceptedFormats[f] = struct{}{}
	}

	return nil
}

func (h *FFMpegHandler) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	const (
		// TODO externalize
		mp4Args = "-vcodec libx264 -preset medium -acodec aac -strict -2 -ab 128k -ac 2 -async 1 -movflags frag_keyframe+empty_moov"
	)

	if ctx.Value(api.MsgDestination).(string) != config.HomarusDestination {
		return ctx, nil
	}

	// Set a default mime type (parity with PHP controller)
	if b.Attachment.Content.MimeType == "" {
		b.Attachment.Content.MimeType = h.DefaultMediaType
	}

	// Map the requested IANA media type to a supported FFmpeg output format
	var outputFormat string
	if format, ok := h.AcceptedFormatsMap[b.Attachment.Content.MimeType]; !ok {
		return ctx, fmt.Errorf("handler: ffmpeg does not support mime type '%s'", b.Attachment.Content.MimeType)
	} else {
		outputFormat = format
	}

	// Apply special arguments for the mp4 output format
	if "mp4" == outputFormat {
		if len(strings.TrimSpace(b.Attachment.Content.Args)) > 0 {
			b.Attachment.Content.Args = fmt.Sprintf("%s %s", strings.TrimSpace(b.Attachment.Content.Args), mp4Args)
		} else {
			b.Attachment.Content.Args = mp4Args
		}
	}

	// command string from Homarus PHP controller
	// $cmd_string = "$this->executable -headers $headers -i $source  $args $cmd_params -f $format -";

	// Manually compose the command arguments so that the additional args supplied on the message
	// are properly parsed
	var cmdArgs []string
	cmdArgs = append(cmdArgs, h.CommandPath)
	//cmdArgs = append(cmdArgs, "-loglevel", "debug")
	cmdArgs = append(cmdArgs, "-headers", fmt.Sprintf("Authorization: %s", asBearer(t)))
	cmdArgs = append(cmdArgs, "-i", b.Attachment.Content.SourceUri)
	if trimmedArgs := strings.TrimSpace(b.Attachment.Content.Args); len(trimmedArgs) > 0 {
		for _, addlArg := range strings.Split(trimmedArgs, " ") {
			cmdArgs = append(cmdArgs, addlArg)
		}
	}
	cmdArgs = append(cmdArgs, "-f", outputFormat)
	cmdArgs = append(cmdArgs, "-")
	cmd := &exec.Cmd{
		Path: "/usr/local/bin/ffmpeg",
		Args: cmdArgs,
	}

	// Compose a PUT request to Drupal, and stream the output of FFmpeg as the PUT request body

	// GET the original image from Drupal
	// Stream the original image into FFmpeg
	// PUT the output of convert (i.e. the derivative) to Drupal
	var (
		ffmpegStdout io.ReadCloser
		err          error
	)

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
	log.Printf("Executing %+v\n", cmd)
	if err := cmd.Start(); err != nil {
		return ctx, err
	}

	// PUT the derivative to Drupal, using stdout from ffmpeg
	_, err = putResourceStream(b.Attachment.Content.DestinationUri, t, ffmpegStdout, map[string]string{
		"Content-Location": b.Attachment.Content.UploadUri,
		"Content-Type":     b.Attachment.Content.MimeType,
	})

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

func getResourceStream(uri string, authToken *jwt.Token, headers map[string]string) (io.ReadCloser, error) {
	var (
		statusCode   int
		statusMsg    string
		responseBody io.ReadCloser
		err          error
	)

	if responseBody, statusCode, statusMsg, err = doRequest(http.MethodGet, uri, authToken, nil, headers); err != nil {
		return nil, err
	}

	if statusCode < 200 || statusCode >= 300 {
		defer func() {
			io.Copy(ioutil.Discard, responseBody)
			responseBody.Close()
		}()
		return responseBody, fmt.Errorf("handler: error performing GET %s: status code '%d', message: '%s'",
			uri, statusCode, statusMsg)
	}

	return responseBody, nil
}

func putResourceStream(uri string, authToken *jwt.Token, body io.ReadCloser, headers map[string]string) (int, error) {
	var (
		statusCode   int
		statusMsg    string
		responseBody io.ReadCloser
		err          error
	)

	if responseBody, statusCode, statusMsg, err = doRequest(http.MethodPut, uri, authToken, body, headers); err != nil {
		return statusCode, err
	} else {
		defer func() {
			io.Copy(ioutil.Discard, responseBody)
			responseBody.Close()
		}()
	}

	if statusCode < 200 || statusCode >= 300 {
		return statusCode, fmt.Errorf("handler: error performing PUT %s: status code '%d', message: '%s'",
			uri, statusCode, statusMsg)
	}

	return statusCode, nil
}

func doRequest(method, uri string, authToken *jwt.Token, body io.ReadCloser, headers map[string]string) (responseBody io.ReadCloser, statusCode int, statusMessage string, err error) {
	var (
		client = http.Client{}

		req *http.Request
		res *http.Response
	)

	req, err = http.NewRequest(method, uri, body)
	if err != nil {
		return nil, -1, "", err
	} else {
		req.Close = true
		if authToken != nil {
			req.Header.Set("Authorization", asBearer(authToken))
		}
		for header, value := range headers {
			req.Header.Set(header, value)
		}
	}

	if res, err = client.Do(req); err != nil {
		return nil, -1, "", err
	}

	return res.Body, res.StatusCode, res.Status, nil
}

func asBearer(token *jwt.Token) string {
	return fmt.Sprintf("Bearer %s", token)
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
	mid := ctx.Value(api.MsgId)
	privateKey := []byte(env.GetOrDefault(config.VarDrupalJwtPrivateKey, ""))
	publicKey := []byte(env.GetOrDefault(config.VarDrupalJwtPublicKey, ""))

	err = verify(t, privateKey, publicKey)

	if err != nil {
		log.Printf("[%s] [%s] handler: JWT could not be verified: %s", "JWTLoggingHandler", mid, err)
	} else {
		log.Printf("[%s] [%s] handler: JWT verified", "JWTLoggingHandler", mid)
	}

	// Decode all claims and log them
	claims := make(map[string]interface{})
	if err := t.DecodeClaims(&claims); err != nil {
		log.Printf("[%s] [%s] handler: error decoding JWT claims: %s", "JWTLoggingHandler", mid, err)
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

	log.Printf("[%s] [%s] handler: %s", "JWTLoggingHandler", mid, builder.String())
	if !expired.IsZero() {
		log.Printf("[%s] [%s] handler: JWT expired at %s", "JWTLoggingHandler", mid, expired.Format(time.RFC3339))
	}

	return ctx, nil
}

func (h *JWTHandler) Handle(ctx context.Context, t *jwt.Token, m *api.MessageBody) (context.Context, error) {
	// TODO: handle a nil token, congruent with handler configuration
	mId := ctx.Value(api.MsgId)

	// FIXME: we don't need a public key for RS256, and we may not need a "private" key for other algorithms.
	//  Figure out appropriate variable names, but we shouldn't panic until we know what keys are needed from the
	//  environment
	var publicKey = []byte(env.GetOrDefault(config.VarDrupalJwtPublicKey, ""))
	var privateKey = []byte(env.GetOrDefault(config.VarDrupalJwtPrivateKey, ""))
	var err error

	err = verify(t, privateKey, publicKey)

	if err != nil {
		return ctx, fmt.Errorf("handler: unable to verify JWT for message-id %s: %w",
			mId, err)
	}

	// Decode registered claims and check expiration
	rClaims := jwt.RegisteredClaims{}

	if err := t.DecodeClaims(&jwt.RegisteredClaims{}); err != nil {
		return ctx, fmt.Errorf("handler: error decoding JWT claims for message-id '%s': %w", mId, err)
	} else if !rClaims.IsValidExpiresAt(time.Now()) {
		return ctx, fmt.Errorf("handler: JWT for message-id %s is expired on %s", mId, rClaims.ExpiresAt.Format(time.RFC3339))
	}

	ctx = context.WithValue(ctx, api.MsgJwt, t)

	log.Printf("[%s] [%s] handler: verified JWT for message %s", "JWTHandler", mId, mId)
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
