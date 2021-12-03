package listener

import (
	"context"
	"derivative-ms/env"
	"errors"
	"fmt"
	"github.com/go-stomp/stomp/v3"
	"log"
	"time"
)

const (
	defaultHost    = "localhost"
	defaultPort    = 61613
	defaultAckMode = "client"
	defaultUser    = ""
	defaultTimeout = 30 * time.Second

	HomarusDestination   = "/queue/islandora-connector-homarus"
	HoudiniDestination   = "/queue/islandora-connector-houdini"
	HypercubeDestination = "/queue/islandora-connector-ocr"

	// MsgJwt keys the JWT found on the message as a *jwt.Token
	MsgJwt = "msg.jwt"

	// MsgDestination keys the message destination as a string, which identifies a STOMP queue
	MsgDestination = "msg.destination"

	// MsgBody keys the *MessageBody, which is provided as a parameter to every Handler
	MsgBody = "msg.body"

	// msgFullBody keys the full message body as a map[string]interface{}
	msgFullBody = "msg.fullBody"

	VarDialTimeoutSeconds = "DERIVATIVE_DIAL_TIMEOUT_SECONDS"
)

type StompListener struct {
	Host          string
	Port          int
	User, Pass    string
	Queue         string
	AckMode       string
	StompHandler  StompHandler
}

type HandlerConfig struct {
	ConfigKey string
}

type StompHandler interface {
	Handle(ctx context.Context, m *stomp.Message) (context.Context, error)
}

type Handler interface {
	Handle(ctx context.Context, b *MessageBody) (context.Context, error)
}

func (l *StompListener) Listen(h Handler) error {
	if l.Queue == "" {
		return errors.New("listener: missing queue")
	}

	setListenerDefaults(l)
	stomp.ConnOpt.Host(l.Host)
	stomp.ConnOpt.Login(l.User, l.Pass)

	var (
		c   *stomp.Conn
		err error
	)

	if c, err = l.dialWithTimeout(env.GetIntOrDefault(VarDialTimeoutSeconds, 30)); err != nil {
		return err
	} else {
		log.Printf("listener: successfully connected to %s:%d, reading messages from %s", l.Host, l.Port, l.Queue)
	}

	var stompAckMode stomp.AckMode
	switch l.AckMode {
	case "client":
		stompAckMode = stomp.AckClientIndividual
	default:
		log.Fatalf("listener: unknown or unsupported ack mode '%s'", l.AckMode)
	}

	subscription, err := c.Subscribe(l.Queue, stompAckMode)
	if err != nil {
		return err
	}

	// provide the subscription to the handler?  Or provide a channel to the handler?
	var ctx context.Context
	for moo := range subscription.C {

		// Privileged handlers that have access to the stomp message
		if ctx, err = l.StompHandler.Handle(context.Background(), moo); err != nil {
			log.Printf("listener: error handling message: %s", err)
			if nackErr := c.Nack(moo); nackErr != nil {
				log.Printf("listener: error nacking message: %s: %s", err, moo.Header.Get("message-id"))
			}
			continue
		}

		if _, err = h.Handle(ctx, ctx.Value(MsgBody).(*MessageBody)); err != nil {
			log.Printf("listener: error handling message: %s", err)
			if nackErr := c.Nack(moo); nackErr != nil {
				log.Printf("listener: error nacking message: %s: %s", err, moo.Header.Get("message-id"))
			}
			continue
		}

		// TODO: what if no handler handled the message

		if moo.ShouldAck() {
			c.Ack(moo)
		}
	}

	return nil
}

func setListenerDefaults(l *StompListener) {
	if l.Host == "" {
		l.Host = defaultHost
	}

	if l.Port == 0 {
		l.Port = defaultPort
	}

	if l.AckMode == "" {
		l.AckMode = defaultAckMode
	}

	if l.User == "" {
		l.User = defaultUser
	}

	if l.Pass == "" {
		l.Pass = ""
	}
}

func (l *StompListener) dialWithTimeout(timeoutSeconds int64) (*stomp.Conn, error) {
	var c *stomp.Conn
	var err error

	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)

	for attempts := 0; time.Now().Before(deadline); attempts++ {
		if c, err := stomp.Dial("tcp", fmt.Sprintf("%s:%d", l.Host, l.Port)); err == nil {
			return c, nil
		}

		time.Sleep(time.Second << uint(attempts))
	}

	return c, fmt.Errorf("listener: timeout expired after %d seconds attempting to dial %s:%d; underlying error was: %w", timeoutSeconds, l.Host, l.Port, err)
}
