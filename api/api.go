package api

import (
	"context"
	"github.com/cristalhq/jwt/v4"
	"io"
	"time"
)

const (
	// MsgJwt keys the JWT found on the message as a *jwt.Token, which is provided as a parameter to every api.Handler
	MsgJwt = "msg.jwt"
	// MsgDestination keys the message destination as a string, which identifies a STOMP queue
	MsgDestination = "msg.destination"
	// MsgBody keys the *MessageBody, which is provided as a parameter to every api.Handler
	MsgBody = "msg.body"
	// MsgFullBody keys the full message body as a map[string]interface{}
	MsgFullBody = "msg.fullBody"
	// MsgId keys the message id
	MsgId = "msg.id"

	Stomp = "stomp"

	Client = "client"
)

type Proto string

type AckMode string

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

type Listener interface {
	Listen(ctx context.Context, handlers []Handler) error
}

type Handler interface {
	Handle(ctx context.Context, t *jwt.Token, b *MessageBody) (context.Context, error)
}

type Dialer interface {
	Dial(host string, port int, timeout time.Duration) (Connection, error)
}

type Subscriber interface {
	Subscribe(queue string, ackMode AckMode) error
}

type Connection interface {
	io.Closer
}