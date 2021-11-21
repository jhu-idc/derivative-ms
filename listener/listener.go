package listener

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-stomp/stomp/v3"
	"log"
)

const (
	defaultHost    = "localhost"
	defaultPort    = 61613
	defaultAckMode = "client"
	defaultUser    = ""

	HomarusDestination = "/queue/islandora-connector-homarus"
)

type StompListener struct {
	Host       string
	Port       int
	User, Pass string
	Queue      string
	AckMode    string
}

type Handler interface {
	Handle(ctx context.Context, m *stomp.Message) (context.Context, error)
}

func (l *StompListener) Listen(h Handler) error {
	if l.Queue == "" {
		return errors.New("listener: missing queue")
	}

	setListenerDefaults(l)
	stomp.ConnOpt.Host(l.Host)
	stomp.ConnOpt.Login(l.User, l.Pass)

	c, err := stomp.Dial("tcp", fmt.Sprintf("%s:%d", l.Host, l.Port))
	if err != nil {
		return err
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
	ctx := context.Background()
	for moo := range subscription.C {
		if _, err := h.Handle(ctx, moo); err != nil {
			log.Printf("%s", err)
			if nackErr := c.Nack(moo); nackErr != nil {
				log.Printf("listener: error nacking message: %s: %s", err, moo.Header.Get("message-id"))
			}
		}

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
