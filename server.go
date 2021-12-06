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
	order       = "order"
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
		handlerConfigs []config.Configuration
		stompHandlers  []listener.StompHandler
		bodyHandlers   []listener.Handler
		err            error
	)

	// Create a config.Configuration for each handler in the application configuration file.
	for configKey, value := range appConfig.Json {
		if configVal, ok := value.(map[string]interface{}); !ok {
			log.Fatalf("error configuring %s: configuration key %s was expected to reference a %T, but was %T",
				os.Args[0], configKey, map[string]interface{}{}, configVal)
		} else {
			if _, ok := configVal[handlerType]; !ok {
				log.Fatalf("error configuring %s: configuration for key %s is missing required value for 'type'",
					os.Args[0], configKey)
			}

			handlerConfig := config.Configuration{
				Key:    configKey,
				Config: appConfig,
				Order:  int(configVal[order].(float64)),
				Type:   configVal[handlerType].(string),
			}

			handlerConfigs = append(handlerConfigs, handlerConfig)
		}
	}

	// Sort the configurations by their order, ascending
	config.Order(handlerConfigs)

	// Configure the sorted handlers, keeping STOMP message handlers separate from STOMP message body handlers.
	for _, handlerConfig := range handlerConfigs {
		var h interface{}
		switch handlerConfig.Type {
		case "JWTLoggingHandler":
			h = &listener.JWTLoggingHandler{}
		case "JWTHandler":
			h = &listener.JWTHandler{}
		case "MessageBody":
			h = &listener.MessageBody{}
		case "StompLoggerHandler":
			h = &listener.StompLoggerHandler{}
		case "Pdf2TextHandler":
			h = &handler.Pdf2TextHandler{}
		case "TesseractHandler":
			h = &handler.TesseractHandler{}
		case "FFMpegHandler":
			h = &handler.FFMpegHandler{}
		case "ImageMagickHandler":
			h = &handler.ImageMagickHandler{}
		default:
			log.Fatalf("error configuring %s: unknown handler configuration type %s", os.Args[0], handlerConfig.Type)
		}

		log.Printf("Configuring %s %T", handlerConfig.Key, h)
		if configurable, ok := h.(config.Configurable); ok {
			if err = configurable.Configure(handlerConfig); err != nil {
				log.Fatalf("error configuring handler %s: %s", handlerConfig.Key, err)
			}
		}

		log.Printf("activating handler: %s", handlerConfig.Key)

		switch t := h.(type) {
		case listener.StompHandler:
			stompHandlers = append(stompHandlers, h.(listener.StompHandler))
		case listener.Handler:
			bodyHandlers = append(bodyHandlers, h.(listener.Handler))
		default:
			log.Fatalf("error configuring %s: unknown handler type %T", handlerConfig.Key, t)
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
