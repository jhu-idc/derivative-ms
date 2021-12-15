package listen

import (
	"context"
	"derivative-ms/api"
	"derivative-ms/api/stomp"
	"fmt"
	"time"
)

type HandlerConfig struct {
	ConfigKey string
}

type ListenerConfig struct {
	BrokerHost  string
	BrokerPort  int
	DialTimeout time.Duration
	Queue       string
	User, Pass  string
	AckMode     api.AckMode
	Proto       api.Proto
	Verbose     bool
}

func Listen(lc *ListenerConfig, handlers []api.Handler) error {
	if lc.Proto != api.Stomp {
		return fmt.Errorf("listener: unsupported protocol '%s'", lc.Proto)
	}

	stompListener := &stomp.ListenerImpl{
		Host:    lc.BrokerHost,
		Port:    lc.BrokerPort,
		User:    lc.User,
		Pass:    lc.Pass,
		Queue:   lc.Queue,
		AckMode: string(lc.AckMode),
		Debug:   lc.Verbose,
	}

	if conn, err := asDialer(stompListener).Dial(lc.BrokerHost, lc.BrokerPort, lc.DialTimeout); err != nil {
		return err
	} else {
		defer conn.Close()
	}

	if err := asSubscriber(stompListener).Subscribe(lc.Queue, lc.AckMode); err != nil {
		return err
	}

	return asListener(stompListener).Listen(context.Background(), handlers)
}

func asDialer(l *stomp.ListenerImpl) api.Dialer {
	return l
}

func asSubscriber(l *stomp.ListenerImpl) api.Subscriber {
	return l
}

func asListener(l *stomp.ListenerImpl) api.Listener {
	return l
}
