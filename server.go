package main

import (
	"derivative-ms/handler"
	"derivative-ms/listener"
	"flag"
	"log"
	"os"
)

const (
	defaultHost = "localhost"
	defaultPort = 61613
	defaultAckMode = "client"

	argQueue  = "queue"
	argBroker = "host"
	argPort   = "port"
	argUser   = "user"
	argPass   = "pass"
	argAckMode = "ack"
)

var brokerHost *string
var brokerPort *int
var queue *string
var user *string
var pass *string
var ackMode *string

func main() {
	// listen mode: hostname, port, client queue
	brokerHost = flag.String(argBroker, defaultHost, "STOMP broker host name")
	brokerPort = flag.Int(argPort, defaultPort, "STOMP broker port")
	queue = flag.String(argQueue, "", "Queue to read messages from")
	user = flag.String(argUser, "", "STOMP broker user name")
	pass = flag.String(argPass, "", "STOMP broker password")
	ackMode = flag.String(argAckMode, defaultAckMode, "STOMP acknowledgment mode")
	flag.Parse()

	l := listener.StompListener{
		Host:    *brokerHost,
		Port:    *brokerPort,
		User:    *user,
		Pass:    *pass,
		Queue:   *queue,
		AckMode: *ackMode,
	}

	writerH := handler.StdOutHandler{os.Stdout}
	jwtVerifierH := handler.JWTHandler{
		RejectIfTokenMissing: true,
		VerifyTokens:         true,
	}
	ffmpegH := handler.FFMpegHandler{}

	err := l.Listen(handler.CompositeHandler{[]listener.Handler{writerH, jwtVerifierH, ffmpegH}})
	if err != nil {
		log.Fatalf("server: exiting with error '%s'", err)
	}

	os.Exit(0)
}
