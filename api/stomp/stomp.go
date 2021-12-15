package stomp

import (
	"context"
	"derivative-ms/api"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cristalhq/jwt/v4"
	"github.com/go-stomp/stomp/v3"
	"log"
	"strings"
	"time"
)

const (
	msgHeaderMessageId = "message-id"
)

type stompHandler interface {
	handle(ctx context.Context, m *stomp.Message) (context.Context, error)
}

type ListenerImpl struct {
	Host       string
	Port       int
	User, Pass string
	Queue      string
	AckMode    string
	Debug      bool

	conn *stomp.Conn
	sub  *stomp.Subscription
}

var Jwt = func(message interface{}) (*jwt.Token, error) {
	var rawToken []byte
	m := message.(*stomp.Message)

	if authHeader := m.Header.Get("Authorization"); authHeader == "" {
		return nil, nil
	} else {
		if strings.HasPrefix(authHeader, "Bearer ") {
			rawToken = []byte(authHeader[len("Bearer "):])
		} else {
			rawToken = []byte(authHeader)
		}
	}

	return jwt.ParseNoVerify(rawToken)
}

var Body = func(message interface{}) (*api.MessageBody, error) {
	m := message.(*stomp.Message)
	instance := &api.MessageBody{}

	if err := json.Unmarshal(m.Body, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

type messageLogger struct {}

type messageIdHandler struct {}

type bodyHandler struct {}

type jwtHandler struct {}

func (*messageIdHandler) handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	return context.WithValue(ctx, api.MsgId, m.Header.Get(msgHeaderMessageId)), nil
}

func (*bodyHandler) handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	var err error
	b := map[string]interface{}{}

	if err = json.Unmarshal(m.Body, &b); err != nil {
		return ctx, err
	}

	fullBodyCtx := context.WithValue(ctx, api.MsgFullBody, &b)

	if b, err := Body(m); err != nil {
		return ctx, err
	} else {
		return context.WithValue(fullBodyCtx, api.MsgBody, b), nil
	}
}

func (*messageLogger) handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	var err error
	var prettyB []byte
	b := map[string]interface{}{}

	if err = json.Unmarshal(m.Body, &b); err != nil {
		return ctx, err
	}
	if prettyB, err = json.MarshalIndent(b, "", "  "); err != nil {
		return ctx, err
	}

	headers := strings.Builder{}
	for i := 0; i < m.Header.Len(); i++ {
		k, v := m.Header.GetAt(i)
		headers.WriteString(fmt.Sprintf("    %s: %s\n", k, v))
	}

	log.Printf("[%s] [%s] STOMP headers and body\n"+
		"  Content Type: %s\n"+
		"  Destination: %s\n"+
		"  Subscription: %s\n"+
		"  Headers:\n%s"+
		"  Body:\n"+
		"%+v\n",
		"StompLoggerHandler", m.Header.Get(msgHeaderMessageId),
		m.ContentType,
		m.Destination,
		m.Subscription.Id(),
		headers.String(),
		string(prettyB))

	return ctx, nil
}

func (jwtHandler) handle(ctx context.Context, m *stomp.Message) (context.Context, error) {
	if t, err := Jwt(m); err != nil {
		return ctx, err
	} else {
		return context.WithValue(ctx, api.MsgJwt, t), nil
	}
}

func (l *ListenerImpl) Dial(host string, port int, timeout time.Duration) (api.Connection, error) {
	stomp.ConnOpt.Host(host)
	stomp.ConnOpt.Login(l.User, l.Pass)

	if c, err := dialWithTimeout(timeout, host, port); err != nil {
		return nil, err
	} else {
		l.conn = c
	}

	log.Printf("stomp: successfully connected to %s:%d", host, port)
	return l, nil
}

func (l *ListenerImpl) Close() error {
	if l.conn != nil {
		if err := l.conn.Disconnect(); err != nil {
			return l.conn.MustDisconnect()
		}
	}

	return nil
}

func (l *ListenerImpl) Subscribe(queue string, ackMode api.AckMode) error {
	var (
		s *stomp.Subscription
		e error
	)
	switch ackMode {
	case api.Client:
		s, e = l.conn.Subscribe(queue, stomp.AckClientIndividual)
	default:
		return fmt.Errorf("stomp: unknown or unsupported acknowledgement mode '%v'", ackMode)
	}

	if e != nil {
		return e
	}

	l.sub = s

	return nil
}

func (l *ListenerImpl) Listen(ctx context.Context, handlers []api.Handler) error {
	if l.Queue == "" {
		return errors.New("stomp: missing queue")
	}

	var stompHandlers []stompHandler

	if l.Debug {
		stompHandlers = append(stompHandlers, &messageLogger{})
	}
	stompHandlers = append(stompHandlers, &messageIdHandler{}, &jwtHandler{}, &bodyHandler{})

	return doSubscribe(l, ctx, stompHandlers, handlers)
}

func doSubscribe(l *ListenerImpl, ctx context.Context, stompHandlers []stompHandler, handlers []api.Handler) (err error) {
	for stompMsg := range l.sub.C {

		// Run internal stomp message handlers first, so they can set the proper state on the context
		for _, h := range stompHandlers {
			if ctx, err = h.handle(ctx, stompMsg); err != nil {
				log.Printf("stomp: error handling message: %s", err)
				if nackErr := l.conn.Nack(stompMsg); nackErr != nil {
					log.Printf("stomp: error nacking message: %s: %s", err, stompMsg.Header.Get(msgHeaderMessageId))
				}
				continue
			}
		}

		body := ctx.Value(api.MsgBody)
		token := ctx.Value(api.MsgJwt)

		// a body is required, the jwt may be optional
		if body == nil {
			l.conn.Nack(stompMsg)
			continue
		}

		// execute publicly configured handlers
		for _, h := range handlers {
			if ctx, err = h.Handle(ctx, token.(*jwt.Token), body.(*api.MessageBody)); err != nil {
				log.Printf("stomp: error handling message: %s", err)
				if nackErr := l.conn.Nack(stompMsg); nackErr != nil {
					log.Printf("stomp: error nacking message: %s: %s", err, stompMsg.Header.Get(msgHeaderMessageId))
				}
				continue
			}
		}

		// TODO: what if no handler handled the message

		if stompMsg.ShouldAck() {
			l.conn.Ack(stompMsg)
		}
	}

	return
}

func dialWithTimeout(timeout time.Duration, host string, port int) (*stomp.Conn, error) {
	var c *stomp.Conn
	var err error

	deadline := time.Now().Add(timeout)

	for attempts := 0; time.Now().Before(deadline); attempts++ {
		if c, err = stomp.Dial("tcp", fmt.Sprintf("%s:%d", host, port)); err == nil {
			return c, nil
		}

		time.Sleep(time.Second << uint(attempts))
	}

	return c, fmt.Errorf("stomp: timeout expired after %d seconds attempting to dial %s:%d; %w", int(timeout.Seconds()), host, port, err)
}
