package handler

import (
	"bufio"
	"context"
	"derivative-ms/config"
	"derivative-ms/listener"
	"fmt"
	"github.com/cristalhq/jwt/v4"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

type CompositeHandler struct {
	Handlers []listener.Handler
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

func (h *TesseractHandler) Handle(ctx context.Context, b *listener.MessageBody) (context.Context, error) {
	if ctx.Value(listener.MsgDestination).(string) != listener.HypercubeDestination {
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
	sourceStream, err = getResourceStream(b.Attachment.Content.SourceUri, extractToken(ctx), nil)
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

	_, err = putResourceStream(b.Attachment.Content.DestinationUri, extractToken(ctx), tStdout, map[string]string{
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

	if h.CommandPath, err = c.StringValue(handlerConfig, "commandPath"); err != nil {
		return fmt.Errorf("handler: unable to configure TesseractHandler '%s', parameter '%s': %w", h.Key, "commandPath", err)
	}

	return nil
}

func (h *Pdf2TextHandler) Handle(ctx context.Context, b *listener.MessageBody) (context.Context, error) {
	if ctx.Value(listener.MsgDestination).(string) != listener.HypercubeDestination {
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
	sourceStream, err = getResourceStream(b.Attachment.Content.SourceUri, extractToken(ctx), nil)
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

	_, err = putResourceStream(b.Attachment.Content.DestinationUri, extractToken(ctx), tStdout, map[string]string{
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

	if h.CommandPath, err = c.StringValue(handlerConfig, "commandPath"); err != nil {
		return fmt.Errorf("handler: unable to configure Pdf2TextHandler '%s', parameter '%s': %w", h.Key, "commandPath", err)
	}

	return nil
}

func (h *ImageMagickHandler) Handle(ctx context.Context, b *listener.MessageBody) (context.Context, error) {
	if ctx.Value(listener.MsgDestination).(string) != listener.HoudiniDestination {
		return ctx, nil
	}

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
	sourceStream, err = getResourceStream(b.Attachment.Content.SourceUri, extractToken(ctx), nil)
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
	_, err = putResourceStream(b.Attachment.Content.DestinationUri, extractToken(ctx), imgStdout, map[string]string{
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

	if h.CommandPath, err = c.StringValue(convertConfig, "commandPath"); err != nil {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler '%s', parameter '%s': %w", h.Key, "commandPath", err)
	}

	if h.DefaultMediaType, err = c.StringValue(convertConfig, "defaultMediaType"); err != nil {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler '%s', parameter '%s': %w", h.Key, "defaultMediaType", err)
	}

	if formats, err = c.SliceStringValue(convertConfig, "acceptedFormats"); err != nil {
		return fmt.Errorf("handler: unable to configure ImageMagickHandler '%s', parameter '%s': %w", h.Key, "acceptedFormats", err)
	}

	h.AcceptedFormats = make(map[string]struct{})

	for _, f := range formats {
		h.AcceptedFormats[f] = struct{}{}
	}

	return nil
}

func (h *FFMpegHandler) Handle(ctx context.Context, b *listener.MessageBody) (context.Context, error) {
	const (
		// TODO externalize
		mp4Args = "-vcodec libx264 -preset medium -acodec aac -strict -2 -ab 128k -ac 2 -async 1 -movflags frag_keyframe+empty_moov"
	)

	if ctx.Value(listener.MsgDestination).(string) != listener.HomarusDestination {
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
	cmdArgs = append(cmdArgs, "-headers", fmt.Sprintf("Authorization: %s", asBearer(extractToken(ctx))))
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
	_, err = putResourceStream(b.Attachment.Content.DestinationUri, extractToken(ctx), ffmpegStdout, map[string]string{
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

	if h.CommandPath, err = c.StringValue(ffmpegConfig, "commandPath"); err != nil {
		return fmt.Errorf("handler: unable to configure FFMpegHandler '%s', parameter '%s': %w", h.Key, "commandPath", err)
	}

	if h.DefaultMediaType, err = c.StringValue(ffmpegConfig, "defaultMediaType"); err != nil {
		return fmt.Errorf("handler: unable to configure FFMpegHandler '%s', parameter '%s': %w", h.Key, "defaultMediaType", err)
	}

	if formats, err = c.MapValue(ffmpegConfig, "acceptedFormatsMap"); err != nil {
		return fmt.Errorf("handler: unable to configure FFMpegHandler '%s', parameter '%s': %w", h.Key, "acceptedFormatsMap", err)
	}

	h.AcceptedFormatsMap = make(map[string]string)

	for k, v := range formats {
		h.AcceptedFormatsMap[k] = v.(string)
	}

	return nil
}

func (h CompositeHandler) Handle(ctx context.Context, b *listener.MessageBody) (context.Context, error) {
	var err error

	for _, handler := range h.Handlers {
		if ctx, err = handler.Handle(ctx, b); err != nil {
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

func extractToken(ctx context.Context) *jwt.Token {
	return ctx.Value(listener.MsgJwt).(*jwt.Token)
}

func asBearer(token *jwt.Token) string {
	return fmt.Sprintf("Bearer %s", token)
}
