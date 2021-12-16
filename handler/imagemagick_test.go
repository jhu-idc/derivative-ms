package handler

import (
	"derivative-ms/api"
	"derivative-ms/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/fs"
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

type suite struct {
	// the context.Context provided to the Handler
	ctx ctxStruct
	// the configuration of the Handler
	configuration config.Configuration
	// a client that mocks GETs and PUTs to Drupal
	drupalClient *mockDrupal
}

type imSuite struct {
	suite
	handlerUnderTest *ImageMagickHandler
}

type ffmpegSuite struct {
	suite
	handlerUnderTest *FFMpegHandler
}

func Test_ImageMagick_Suite(t *testing.T) {
	suite, drupalClient := newImageMagickSuite()
	require.Nil(t, suite.handlerUnderTest.configure(suite.configuration, true))

	t.Run("CommandNotFound", func(t *testing.T) {
		suite.handlerUnderTest.CommandPath = "moo"
		_, err := suite.handlerUnderTest.Handle(suite.ctx.ctx, nil, &api.MessageBody{})
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	// Reset suite state between tests
	suite, drupalClient = newImageMagickSuite()
	require.Nil(t, suite.handlerUnderTest.configure(suite.configuration, true))

	t.Run("ExecOk", func(t *testing.T) {
		echoPath, err := exec.LookPath("echo")
		require.Nil(t, err)
		suite.handlerUnderTest.CommandBuilder = &mockCmd{cmd: &exec.Cmd{
			Path: echoPath,
			Args: []string{echoPath, "hello world"},
		}}

		_, err = suite.handlerUnderTest.Handle(suite.ctx.ctx, nil, &api.MessageBody{})
		assert.Nil(t, err)
		assert.Equal(t, []byte("hello world\n"), drupalClient.put.body)
	})
}

func Test_FFMpeg_Suite(t *testing.T) {
	suite, drupalClient := newFFMpegSuite()
	require.Nil(t, suite.handlerUnderTest.configure(suite.configuration, true))

	t.Run("CommandNotFound", func(t *testing.T) {
		suite.handlerUnderTest.CommandPath = "moo"
		_, err := suite.handlerUnderTest.Handle(suite.ctx.ctx, nil, &api.MessageBody{})
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	// Reset suite state between tests
	suite, drupalClient = newFFMpegSuite()
	require.Nil(t, suite.handlerUnderTest.configure(suite.configuration, true))

	t.Run("ExecOk", func(t *testing.T) {
		echoPath, err := exec.LookPath("echo")
		require.Nil(t, err)
		suite.handlerUnderTest.CommandBuilder = &mockCmd{cmd: &exec.Cmd{
			Path: echoPath,
			Args: []string{echoPath, "hello world"},
		}}

		_, err = suite.handlerUnderTest.Handle(suite.ctx.ctx, nil, &api.MessageBody{})
		assert.Nil(t, err)
		assert.Equal(t, []byte("hello world\n"), drupalClient.put.body)
	})
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
		handlerUnderTest: &ImageMagickHandler{
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
		handlerUnderTest: &FFMpegHandler{
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
