package drupal

import (
	"derivative-ms/drupal/request"
	"io"
)

type Client interface {
	Put(reqCtx request.Context, uri string, body io.ReadCloser) (int, error)
	Get(reqCtx request.Context, uri string) (io.ReadCloser, error)
}
