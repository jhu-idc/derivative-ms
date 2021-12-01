package main

import (
	"derivative-ms/config"
	"derivative-ms/handler"
	"derivative-ms/listener"
	"flag"
	"log"
	"os"
)

const (
	defaultHost    = "localhost"
	defaultPort    = 61613
	defaultAckMode = "client"

	argQueue   = "queue"
	argBroker  = "host"
	argPort    = "port"
	argUser    = "user"
	argPass    = "pass"
	argAckMode = "ack"
	argConfig  = "config"
)

var (
	brokerHost *string
	brokerPort *int
	queue      *string
	user       *string
	pass       *string
	ackMode    *string
	cliConfig  *string
)

func main() {
	// listen mode: hostname, port, client queue
	brokerHost = flag.String(argBroker, defaultHost, "STOMP broker host name, e.g. 'islandora-idc.traefik.me'")
	brokerPort = flag.Int(argPort, defaultPort, "STOMP broker port")
	queue = flag.String(argQueue, "", "Queue to read messages from, e.g. 'islandora-connector-homarus' or 'ActiveMQ.DLQ'")
	user = flag.String(argUser, "", "STOMP broker user name")
	pass = flag.String(argPass, "", "STOMP broker password")
	ackMode = flag.String(argAckMode, defaultAckMode, "STOMP acknowledgment mode, e.g. 'client' or 'auto'")
	cliConfig = flag.String(argConfig, "", "Path to handler configuration file")
	flag.Parse()
	serviceConfig := &config.Config{}
	serviceConfig.Resolve(*cliConfig)

	var err error

	jwtVerifierH := &listener.JWTHandler{
		Configuration: config.Configuration{
			Key:    "jwt",
			Config: serviceConfig},
	}

	// Essentially the 'jwt' handler is required, so a valid configuration must be present
	if err := jwtVerifierH.Configure(); err != nil {
		log.Fatalf("error configuring stomp handler: %s", err)
	}

	loggerHandler := listener.StompLoggerHandler{Writer: os.Stdout}
	bodyH := listener.MessageBody{}

	l := listener.StompListener{
		Host:    *brokerHost,
		Port:    *brokerPort,
		User:    *user,
		Pass:    *pass,
		Queue:   *queue,
		AckMode: *ackMode,
		StompHandler: listener.CompositeStompHandler{
			Handlers: []listener.StompHandler{loggerHandler, bodyH, jwtVerifierH},
		},
	}

	ffmpegH := &handler.FFMpegHandler{
		Configuration: config.Configuration{
			Key:    "ffmpeg",
			Config: serviceConfig,
		},
	}

	tesseractH := &handler.TesseractHandler{
		Configuration: config.Configuration{
			Key:    "tesseract",
			Config: serviceConfig,
		},
	}

	pdf2txtH := &handler.Pdf2TextHandler{
		Configuration: config.Configuration{
			Key:    "pdf2txt",
			Config: serviceConfig,
		},
	}

	imageH := &handler.ImageMagickHandler{
		Configuration: config.Configuration{
			Key:    "convert",
			Config: serviceConfig},
	}

	candidateHandlers := []listener.Handler{ffmpegH, imageH, tesseractH, pdf2txtH}
	var handlers []listener.Handler

	// Handlers that operate on the body of the message are optional.  If there is a valid configuration present for
	// a handler, then they are included in the runtime.  If there is no valid configuration, we presume that the
	// handler was removed for one reason or another, so we pass over it without considering it a fatal error.
	//
	// TODO: provide for a more robust mechanism of indicating which handlers should be required at runtime, and which
	//   are optional.  Allow for the key of a given handler to be specified or discovered.
	for _, h := range candidateHandlers {
		h := h
		configurable := h.(config.Configurable).Configure()
		if err := configurable; err != nil {
			log.Printf("error configuring handler: %s", err)
		} else {
			handlers = append(handlers, h)
		}
	}

	err = l.Listen(handler.CompositeHandler{Handlers: handlers})
	if err != nil {
		log.Fatalf("server: exiting with error '%s'", err)
	}

	os.Exit(0)
}
