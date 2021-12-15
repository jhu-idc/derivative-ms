package main

import (
	"derivative-ms/api"
	"derivative-ms/config"
	"derivative-ms/env"
	"derivative-ms/handler"
	"derivative-ms/listen"
	"flag"
	"log"
	"os"
	"time"
)

const (
	defaultHost    = "localhost"
	defaultPort    = 61613
	defaultAckMode = "client"
	defaultUser    = ""
	defaultTimeout = 30

	argQueue   = "queue"
	argBroker  = "host"
	argPort    = "port"
	argUser    = "user"
	argPass    = "pass"
	argAckMode = "ack"
	argConfig  = "config"
	argVerbose = "verbose"

	handlerType = "handler-type"
	order       = "order"
)

func main() {
	appConfig := &config.Config{
		Cli: &config.Args{
			BrokerHost:    flag.String(argBroker, defaultHost, "STOMP broker host name, e.g. 'islandora-idc.traefik.me'"),
			BrokerPort:    flag.Int(argPort, defaultPort, "STOMP broker port"),
			Queue:         flag.String(argQueue, "", "Queue to read messages from, e.g. 'islandora-connector-homarus' or 'ActiveMQ.DLQ'"),
			User:          flag.String(argUser, defaultUser, "STOMP broker user name"),
			Pass:          flag.String(argPass, "", "STOMP broker password"),
			AckMode:       flag.String(argAckMode, defaultAckMode, "STOMP acknowledgment mode, e.g. 'client' or 'auto'"),
			CliConfigFile: flag.String(argConfig, "", "Path to handler configuration file"),
			Verbose:       flag.Bool(argVerbose, false, "enable verbose output"),
		},
	}
	flag.Parse()
	appConfig.Resolve(*appConfig.Cli.CliConfigFile)

	var (
		handlerConfigs []config.Configuration
		handlers       []api.Handler
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

	// Configure the sorted handlers
	for _, handlerConfig := range handlerConfigs {
		var h interface{}
		switch handlerConfig.Type {
		case "JWTLoggingHandler":
			h = &handler.JWTLoggingHandler{}
		case "JWTHandler":
			h = &handler.JWTHandler{}
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
		handlers = append(handlers, h.(api.Handler))
	}

	lc := &listen.ListenerConfig{
		BrokerHost:  *appConfig.Cli.BrokerHost,
		BrokerPort:  *appConfig.Cli.BrokerPort,
		DialTimeout: time.Duration(env.GetIntOrDefault(config.VarDialTimeoutSeconds, defaultTimeout)) * time.Second,
		Queue:       *appConfig.Cli.Queue,
		User:        *appConfig.Cli.User,
		Pass:        *appConfig.Cli.Pass,
		AckMode:     api.AckMode(*appConfig.Cli.AckMode),
		Proto:       api.Stomp,
		Verbose:     *appConfig.Cli.Verbose,
	}

	err = listen.Listen(lc, handlers)

	if err != nil {
		log.Fatalf("server: exiting with error %s", err)
	}

	os.Exit(0)
}
