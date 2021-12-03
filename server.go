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

	handlerType = "handler-type"
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
	appConfig := &config.Config{}
	appConfig.Resolve(*cliConfig)

	var (
		stompHandlers []listener.StompHandler
		bodyHandlers  []listener.Handler
		err           error
	)

	for configKey, interfaceVal := range appConfig.Json {
		if configVal, ok := interfaceVal.(map[string]interface{}); !ok {
			log.Fatalf("error configuring %s: configuration key %s was expected to reference a %T, but was %T",
				os.Args[0], configKey, map[string]interface{}{}, configVal)
		} else {
			if _, ok := configVal[handlerType]; !ok {
				log.Fatalf("error configuring %s: configuration for key %s is missing required value for 'type'",
					os.Args[0], configKey)
			}

			var h interface{}

			handlerConfig := config.Configuration{
				Key:    configKey,
				Config: appConfig,
			}

			switch configVal[handlerType] {
			case "JWTLoggingHandler":
				h = &listener.JWTLoggingHandler{}
			case "JWTHandler":
				h = &listener.JWTHandler{}
			case "MessageBody":
				h = &listener.MessageBody{}
			case "StompLoggerHandler":
				h = &listener.StompLoggerHandler{Writer: os.Stdout}
			case "Pdf2TextHandler":
				h = &handler.Pdf2TextHandler{}
			case "TesseractHandler":
				h = &handler.TesseractHandler{}
			case "FFMpegHandler":
				h = &handler.FFMpegHandler{}
			case "ImageMagickHandler":
				h = &handler.ImageMagickHandler{}
			default:
				log.Fatalf("error configuring %s: unknown handler configuration type %s", os.Args[0], configVal[handlerType])
			}

			log.Printf("Configuring %s %T", configKey, h)
			if configurable, ok := h.(config.Configurable); ok {
				if err = configurable.Configure(handlerConfig); err != nil {
					log.Fatalf("error configuring handler %s: %s", configKey, err)
				}
			}

			log.Printf("activating handler: %s", configKey)

			switch t := h.(type) {
			case listener.StompHandler:
				stompHandlers = append(stompHandlers, h.(listener.StompHandler))
			case listener.Handler:
				bodyHandlers = append(bodyHandlers, h.(listener.Handler))
			default:
				log.Fatalf("error configuring %s: unknown handler type %T", configKey, t)
			}
		}
	}

	l := listener.StompListener{
		Host:    *brokerHost,
		Port:    *brokerPort,
		User:    *user,
		Pass:    *pass,
		Queue:   *queue,
		AckMode: *ackMode,
		StompHandler: listener.CompositeStompHandler{
			Handlers: stompHandlers,
		},
	}

	err = l.Listen(handler.CompositeHandler{Handlers: bodyHandlers})
	if err != nil {
		log.Fatalf("server: exiting with error %s", err)
	}

	os.Exit(0)
}
