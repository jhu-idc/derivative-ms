package handler

import (
	"derivative-ms/api"
	"derivative-ms/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/fs"
	"os"
	"os/exec"
	"testing"
)

var imDefaultConfig = map[string]interface{}{
	"defaultMediaType": "image/jpeg",
	"acceptedFormats": []string{
		"image/jpeg",
		"image/png",
		"image/tiff",
		"image/jp2",
	},
}

var ffmpegDefaultConfig = map[string]interface{}{
	"defaultMediaType": "video/mp4",
	"acceptedFormatsMap": map[string]interface{}{
		"video/mp4":       "mp4",
		"video/x-msvideo": "avi",
		"video/ogg":       "ogg",
		"audio/x-wav":     "wav",
		"audio/mpeg":      "mp3",
		"audio/aac":       "m4a",
		"image/jpeg":      "image2pipe",
		"image/png":       "png_image2pipe",
	},
}

var tesseractDefaultConfig = map[string]interface{}{}

var pdf2TextDefaultConfig = map[string]interface{}{}

func Test_ImageMagick_Suite(t *testing.T) {
	suite, _ := newImageMagickSuite()
	require.Nil(t, suite.handler.configure(suite.configuration, true))
	t.Run("CommandNotFound", testCommandNotFound(mutableHandler(suite), &suite.suite))

	// Reset suite state between tests
	suite, _ = newImageMagickSuite()
	require.Nil(t, suite.handler.configure(suite.configuration, true))
	t.Run("ExecOk", testExecOk(mutableHandler(suite), &suite.suite))
}

func Test_FFMpeg_Suite(t *testing.T) {
	suite, _ := newFFMpegSuite()
	require.Nil(t, suite.handler.configure(suite.configuration, true))
	t.Run("CommandNotFound", testCommandNotFound(mutableHandler(suite), &suite.suite))

	// Reset suite state between tests
	suite, _ = newFFMpegSuite()
	require.Nil(t, suite.handler.configure(suite.configuration, true))
	t.Run("ExecOk", testExecOk(mutableHandler(suite), &suite.suite))
}

func Test_Tesseract_Suite(t *testing.T) {
	suite, _ := newTesseractSuite()
	require.Nil(t, suite.handler.configure(suite.configuration, true))
	suite.drupalClient.get.retBody, _ = os.OpenFile("testdata/magic-tif-bytes.bin", os.O_RDONLY, os.FileMode(0775))
	t.Run("CommandNotFound", testCommandNotFound(mutableHandler(suite), &suite.suite))

	// Reset suite state between tests
	suite, _ = newTesseractSuite()
	require.Nil(t, suite.handler.configure(suite.configuration, true))
	suite.drupalClient.get.retBody, _ = os.OpenFile("testdata/magic-tif-bytes.bin", os.O_RDONLY, os.FileMode(0775))
	t.Run("ExecOk", testExecOk(mutableHandler(suite), &suite.suite))
}

func Test_Pdf2Text_Suite(t *testing.T) {
	suite, _ := newPdf2TextSuite()
	require.Nil(t, suite.handler.configure(suite.configuration, true))
	suite.drupalClient.get.retBody, _ = os.OpenFile("testdata/magic-pdf-bytes.bin", os.O_RDONLY, os.FileMode(0775))
	t.Run("CommandNotFound", testCommandNotFound(mutableHandler(suite), &suite.suite))

	// Reset suite state between tests
	suite, _ = newPdf2TextSuite()
	require.Nil(t, suite.handler.configure(suite.configuration, true))
	suite.drupalClient.get.retBody, _ = os.OpenFile("testdata/magic-pdf-bytes.bin", os.O_RDONLY, os.FileMode(0775))
	t.Run("ExecOk", testExecOk(mutableHandler(suite), &suite.suite))
}

func testExecOk(h mutableHandler, s *suite) func(*testing.T) {
	return func(t *testing.T) {
		echoPath, err := exec.LookPath("echo")
		require.Nil(t, err)
		h.setCommandBuilder(&mockCmd{cmd: &exec.Cmd{
			Path: echoPath,
			Args: []string{echoPath, "hello world"},
		}})

		_, err = h.Handle(s.ctx.ctx, nil, &api.MessageBody{})
		assert.Nil(t, err)
		assert.Equal(t, []byte("hello world\n"), s.drupalClient.put.body)
	}
}

func testCommandNotFound(h mutableHandler, s *suite) func(t *testing.T) {
	return func(t *testing.T) {
		h.setCommandPath("moo")
		_, err := h.Handle(s.ctx.ctx, nil, &api.MessageBody{})
		assert.ErrorIs(t, err, fs.ErrNotExist)
	}
}

func newImageMagickSuite() (*imSuite, *mockDrupal) {
	destination := config.HoudiniDestination
	configKey := "convertTest"
	handlerConfig := imDefaultConfig
	messageBody := api.MessageBody{}

	c := config.Configuration{
		Key: configKey,
		Config: &config.Config{
			Json: map[string]interface{}{
				configKey: handlerConfig,
			},
		},
	}

	s, d := newSuite(newContext("moo-msg-id", destination, messageBody), c)

	return &imSuite{
		suite: *s,
		handler: &ImageMagickHandler{
			Configuration: c,
			Drupal:        d,
		},
	}, d
}

func newFFMpegSuite() (*ffmpegSuite, *mockDrupal) {
	destination := config.HomarusDestination
	configKey := "ffmpegTest"
	handlerConfig := ffmpegDefaultConfig
	messageBody := api.MessageBody{}

	c := config.Configuration{
		Key: configKey,
		Config: &config.Config{
			Json: map[string]interface{}{
				configKey: handlerConfig,
			},
		},
	}

	s, d := newSuite(newContext("moo-msg-id", destination, messageBody), c)
	return &ffmpegSuite{
		suite: *s,
		handler: &FFMpegHandler{
			Configuration: c,
			Drupal:        d,
		},
	}, d
}

func newTesseractSuite() (*tesseractSuite, *mockDrupal) {
	destination := config.HypercubeDestination
	configKey := "tesseractTest"
	handlerConfig := tesseractDefaultConfig
	messageBody := api.MessageBody{}

	c := config.Configuration{
		Key: configKey,
		Config: &config.Config{
			Json: map[string]interface{}{
				configKey: handlerConfig,
			},
		},
	}

	s, d := newSuite(newContext("moo-msg-id", destination, messageBody), c)
	return &tesseractSuite{
		suite: *s,
		handler: &TesseractHandler{
			Configuration: c,
			Drupal:        d,
		},
	}, d
}

func newPdf2TextSuite() (*pdf2TextSuite, *mockDrupal) {
	destination := config.HypercubeDestination
	configKey := "pdf2textTest"
	handlerConfig := pdf2TextDefaultConfig
	messageBody := api.MessageBody{}

	c := config.Configuration{
		Key: configKey,
		Config: &config.Config{
			Json: map[string]interface{}{
				configKey: handlerConfig,
			},
		},
	}

	s, d := newSuite(newContext("moo-msg-id", destination, messageBody), c)
	return &pdf2TextSuite{
		suite: *s,
		handler: &Pdf2TextHandler{
			Configuration: c,
			Drupal:        d,
		},
	}, d
}

func newSuite(ctx ctxStruct, c config.Configuration) (*suite, *mockDrupal) {
	drupalClient := &mockDrupal{}
	return &suite{
		// context containing the necessary information extracted from the STOMP message
		ctx: ctx,
		// standard configuration for the handler
		configuration: c,
		// mock drupal client
		drupalClient: drupalClient,
	}, drupalClient
}
