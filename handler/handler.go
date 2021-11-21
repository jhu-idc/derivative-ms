package handler

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"derivative-ms/listener"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/cristalhq/jwt/v4"
	"github.com/go-stomp/stomp/v3"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

// TODO: useful?
type CompositeHandler struct {
	Handlers []listener.Handler
}

type JWTHandler struct {
	RejectIfTokenMissing bool
	VerifyTokens         bool
	Key                  []byte
}

// TODO these handlers need to be selected after parsing the message body
// TODO use plugin mechanism for handlers?
type ImageMagickHandler struct {
}

type FFMpegHandler struct {
}

func (h FFMpegHandler) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	const (
		// TODO externalize
		mp4Args = "-vcodec libx264 -preset medium -acodec aac -strict -2 -ab 128k -ac 2 -async 1 -movflags frag_keyframe+empty_moov"
	)
	var (
		// TODO externalize
		ffmpegFormat = map[string]string{
			"video/mp4":       "mp4",
			"video/x-msvideo": "avi",
			"video/ogg":       "ogg",
			"audio/x-wav":     "wav",
			"audio/mpeg":      "mp3",
			"audio/aac":       "m4a",
			"image/jpeg":      "image2pipe",
			"image/png":       "png_image2pipe",
		}
		outputFormat string
	)

	if m.Destination != listener.HomarusDestination {
		return ctx, nil
	}

	body := struct {
		Attachment struct {
			Content struct {
				Args           string `json:"args,omitempty"`
				DestinationUri string `json:"destination_uri"`
				UploadUri      string `json:"file_upload_uri"`
				MimeType       string `json:"mimetype"`
				SourceUri      string `json:"source_uri"`
			}
		}
	}{}

	if err := json.Unmarshal(m.Body, &body); err != nil {
		return ctx, err
	}

	// Map the requested IANA media type to a supported FFmpeg output format
	if format, ok := ffmpegFormat[body.Attachment.Content.MimeType]; !ok {
		return ctx, fmt.Errorf("handler: ffmpeg does not support mime type '%s'", body.Attachment.Content.MimeType)
	} else {
		outputFormat = format
	}

	// Apply special arguments for the mp4 output format
	if "mp4" == outputFormat {
		if len(strings.TrimSpace(body.Attachment.Content.Args)) > 0 {
			body.Attachment.Content.Args = fmt.Sprintf("%s %s", strings.TrimSpace(body.Attachment.Content.Args), mp4Args)
		} else {
			body.Attachment.Content.Args = mp4Args
		}
	}

	// command string from Homarus PHP controller
	// $cmd_string = "$this->executable -headers $headers -i $source  $args $cmd_params -f $format -";

	//cmd := exec.Command("ffmpeg",
	//	"-headers", fmt.Sprintf("'Authorization: Bearer %s'", ctx.Value("jwt").(*jwt.Token)),
	//	"-i", body.Attachment.Content.SourceUri,
	//	body.Attachment.Content.Args,
	//	"-f", outputFormat,
	//	"-")

	var cmdArgs []string
	cmdArgs = append(cmdArgs, "/usr/local/bin/ffmpeg")
	//cmdArgs = append(cmdArgs, "-loglevel", "debug")
	cmdArgs = append(cmdArgs, "-headers", fmt.Sprintf("'Authorization: Bearer %s'", ctx.Value("jwt").(*jwt.Token)))
	cmdArgs = append(cmdArgs, "-i", body.Attachment.Content.SourceUri)
	for _, addlArg := range strings.Split(body.Attachment.Content.Args, " ") {
		cmdArgs = append(cmdArgs, addlArg)
	}
	cmdArgs = append(cmdArgs, "-f", outputFormat)
	cmdArgs = append(cmdArgs, "-")
	cmd := &exec.Cmd{
		Path: "/usr/local/bin/ffmpeg",
		Args: cmdArgs,
	}

	// Compose a PUT request to Drupal, and stream the output of FFmpeg as the PUT request body
	/*
		// PUT the media.
		            .removeHeaders("*", "Authorization", "Content-Type")
		            .setHeader("Content-Location", simple("${exchangeProperty.event.attachment.content.fileUploadUri}"))
		            .setHeader(Exchange.HTTP_METHOD, constant("PUT"))

		            .log(DEBUG, LOGGER, "Processing response - Service URL: '{{derivative.service.url}}' " +
		                    "Source URI: '${exchangeProperty.event.attachment.content.sourceUri}' File Upload URI: " +
		                    "'${exchangeProperty.event.attachment.content.fileUploadUri}' Destination URI: " +
		                    "'${exchangeProperty.event.attachment.content.destinationUri}'")

		            .toD("${exchangeProperty.event.attachment.content.destinationUri}?connectionClose=true");
		    }
	*/

	var req *http.Request
	var res *http.Response
	var derivativeStream io.ReadCloser
	var err error

	if derivativeStream, err = cmd.StdoutPipe(); err != nil {
		return ctx, err
	}

	//if err = cmd.Start(); err != nil {
	//	return ctx, err
	//}

	//f, _ := ioutil.TempFile("/tmp", "ffmpeg-*")
	//if written, err := io.Copy(f, derivativeStream); err != nil {
	//	return ctx, err
	//} else {
	//	log.Printf("handler: wrote %d bytes to %s", written, f.Name())
	//}

	if req, err = http.NewRequest(http.MethodPut, body.Attachment.Content.DestinationUri, derivativeStream); err != nil {
		return ctx, err
	} else {
		defer req.Body.Close()
		req.Close = true
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ctx.Value("jwt").(*jwt.Token)))
		req.Header.Set("Content-Location", body.Attachment.Content.UploadUri)
		req.Header.Set("Content-Type", body.Attachment.Content.MimeType)
	}

	log.Printf("Executing %+v\n", cmd)
	if err = cmd.Start(); err != nil {
		return ctx, err
	}

	client := http.Client{}
	res, err = client.Do(req)
	defer func() {
		io.ReadAll(res.Body)
		res.Body.Close()
	}()
	if err != nil {
		return ctx, err
	}
	// should get a 201 or 204
	log.Printf("handler: received %d %s from %s %s", res.StatusCode, res.Status, req.Method, req.RequestURI)

	err = cmd.Wait()

	return ctx, err
}

type StdOutHandler struct {
	Writer io.Writer
}

func (h StdOutHandler) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	b := map[string]interface{}{}
	json.Unmarshal(m.Body, &b)
	prettyB, _ := json.MarshalIndent(b, "", "  ")

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

	return context.WithValue(ctx, "body", &b), nil
}

func (h CompositeHandler) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	var err error

	// shadow the context, so it can be passed along the handler chain
	c := ctx
	for _, handler := range h.Handlers {
		if c, err = handler.Handle(c, m); err != nil {
			return c, err
		}
	}

	return c, err
}

func (h JWTHandler) Handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	mId := m.Header.Get("message-id")

	// FIXME (from crayfits /opt/keys/jwt/public.key)
	var publicKey = []byte(`
-----BEGIN RSA PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA6uK3nozywVaRCAB3FHdR
ZNHunSZvN/c31QimZAqQMGxj7JrGh1LF8JRX+XAQ+CJcPD9r6xXjKSS1Gqa2Os2w
ARr/9abIwG5QeNsrJ8GMt3Z/WICnNeaFAkUVviwKWcA61iFJWvTDAuI0hCaxArRK
sk0BfFSMh+4u3JAdD9tUxUx6AAUXUCdtPyluaBd53wuB0r9xRlPnDw6I9QHfKK80
Xrrsu1PYATgrsy69stzCln3KlO5Oxc6O8OjMdjC2D2c3HmsO4CKPvvaVuaow/a9P
a3SNje4UXN+/1xUfQskxafP8CKVSr8xxtwzSureiskb5/98moAiutpUtp15yyAm0
rwIDAQAB
-----END RSA PUBLIC KEY-----
`)
	var privateKey = []byte(`
-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA6uK3nozywVaRCAB3FHdRZNHunSZvN/c31QimZAqQMGxj7JrG
h1LF8JRX+XAQ+CJcPD9r6xXjKSS1Gqa2Os2wARr/9abIwG5QeNsrJ8GMt3Z/WICn
NeaFAkUVviwKWcA61iFJWvTDAuI0hCaxArRKsk0BfFSMh+4u3JAdD9tUxUx6AAUX
UCdtPyluaBd53wuB0r9xRlPnDw6I9QHfKK80Xrrsu1PYATgrsy69stzCln3KlO5O
xc6O8OjMdjC2D2c3HmsO4CKPvvaVuaow/a9Pa3SNje4UXN+/1xUfQskxafP8CKVS
r8xxtwzSureiskb5/98moAiutpUtp15yyAm0rwIDAQABAoIBAA0PZh5OwAC4C4Bi
ZjyhFcmBUr8yL+Twvg3+WSIe5D2NCVFSmc9UbuUdmnaoIIlrf61p6Vo88VCMVfWR
Z3iFj0/AbJMAHxF0EM1nglLHlEdvM018ec+pbaPeq4LTeA/dfGgDmcyQ53b1lO30
KMt5st2PIpIDMX0tZTWmXbdP/rqplqiQmdwH0gv8PzEG6Y2ZVLBf5viH5IvVRpg7
9nDqLfe2W2ylFib+CtleX626xTUzGcJ0aqTRP1UkY4Jj5PI2/yVqttYPkvJtxZco
5/14AEMcu9FMBhADCSk/0y1TkKCGsi6/VNd78AB/RrZK32HfHCwTwxoXHQoaVKq6
hNQfokECgYEA+keGdVJrXMDylamARQdDe/nNgljqZZhfkKKYCGeckNqjp3iqfmld
/tqCPVxAO2mIo8dNNfM2MMv6loEPx9F2REe8k9NbWFrUZ486fMHPeO7WnHt92JyU
DtfJJSZ1GdCki1PthmpmP8WdoF6VpLvr5AgwuYKAzkMNth+OV/dvgqkCgYEA8EEe
E3mKvePHV/PVsLt6TaqJcZEKKu6L9EgeDyzv3zz4+2zG8MVctyUFyfSl4EIC/oJy
X0T5Tj1l4A3mPwZOJfQOkXnr9TaPNff1zjNx12RhUZjFtJU5V+Wn1ldtzs+XwCFc
x5O/B8LWYgV4bOixNlc6tTRq/m8Txvtde9vPa5cCgYEA3OOdnxRD31QHheFYbRPx
Eo0xPNae4VWvGmb2SYywmQPuplMQHot+Qvy1L9SoeAc3alzvHytta3nLy2NS+yc5
+x9ZJxrGJt/bUR8PHqarJu+ch/VR54ih/8uhImGjvknvv2wuWZC0d5pA+RYheofE
tLgp0MCGUATMKC4HokmmqCkCgYBEjoBTlFIn33CJw3WNyeGbefdgZb/eAlYDbfTN
5cfJDvAJZr/aAqdzR2hAecQ/mvaZw4V5dAgj8Fc6uRyjjVwNbngdwQm43km9X7VP
ktSAXw96Jjr8TbygPVNIUYhvBEPMOnjsJlfTkiB0thToFvpChF+nR37kfbPKCv5h
Epc8nwKBgQCdKyLi54Fm24nqEuZYbAxxGI9TVT7wJjoKGn64JWrXtX7xRltmJC3t
nLwNCojcbyG4kVB+Myzr2OEtFkO45j83GjrZ4O+jCuSj+AmCxEcc7xNA9cgu9usG
sQXdGmIIB0Cbk54OyHNdsZgZCXi9GTRF9uvYZKL9qktS+UZMJ1Xz/g==
-----END RSA PRIVATE KEY-----
`)

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
	} else {
		ctx = context.WithValue(ctx, "jwt", token)
	}

	token.String()
	log.Printf("handler: verified JWT for message %s", mId)
	return ctx, nil
}
