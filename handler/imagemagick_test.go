package handler

import (
	"context"
	"derivative-ms/api"
	"derivative-ms/cmd"
	"derivative-ms/config"
	"github.com/cristalhq/jwt/v4"
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
	handler *ImageMagickHandler
}

type ffmpegSuite struct {
	suite
	handler *FFMpegHandler
}

type mutableHandler interface {
	api.Handler
	setCommandBuilder(c cmd.Builder)
	setCommandPath(cmdPath string)
}

func (s imSuite) setCommandBuilder(c cmd.Builder) {
	s.handler.CommandBuilder = c
}

func (s imSuite) setCommandPath(cmd string) {
	s.handler.CommandPath = cmd
}

func (s imSuite) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	return s.handler.Handle(ctx, t, b)
}

func (s ffmpegSuite) setCommandBuilder(c cmd.Builder) {
	s.handler.CommandBuilder = c
}

func (s ffmpegSuite) setCommandPath(cmd string) {
	s.handler.CommandPath = cmd
}

func (s ffmpegSuite) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	return s.handler.Handle(ctx, t, b)
}

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
