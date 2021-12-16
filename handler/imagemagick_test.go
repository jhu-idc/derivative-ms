package handler

import (
	"derivative-ms/api"
	"derivative-ms/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/fs"
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

func Test_ImageMagick_CommandNotFound(t *testing.T) {
	// create a context containing the necessary information extracted from the STOMP message
	ctx := newContext("moo-msg-id", config.HoudiniDestination, api.MessageBody{})

	// create a standard configuration for the handler
	configuration := config.Configuration{
		Key: "convertTest",
		Config: &config.Config{
			Json: map[string]interface{}{
				"convertTest": imDefaultConfig,
			},
		},
	}

	// instantiate and configure the handler, ignoring any configuration errors
	underTest := &ImageMagickHandler{}
	require.Nil(t, underTest.configure(configuration, true))

	// override/set a mock drupal client and a command path that does not exist
	underTest.Drupal = mockDrupal{}
	underTest.CommandPath = "moo"

	_, err := underTest.Handle(ctx.ctx, nil, &api.MessageBody{})
	assert.ErrorIs(t, err, fs.ErrNotExist)
}
