package config

import (
	"derivative-ms/env"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
)

const (
	defaultConfig    = "handlers.json"
	VarHandlerConfig = "DERIVATIVE_HANDLER_CONFIG"
)

var (
	// ParseErr indicates the application configuration could not be parsed
	ParseErr = errors.New("error parsing configuration")
	// TypeConvErr indicates that a type conversion failed
	TypeConvErr = errors.New("error converting type")
	// NotFoundErr indicates that the desired configuration was not found
	NotFoundErr = errors.New("configuration not found")
)

//go:embed *.json
var configFs embed.FS

// Config maintains the application configuration, including the configuration for each Handler.  The Resolve method
// is used to populate this Config.
type Config struct {
	// Embedded is true when the Location refers to a //go:embedded handler configuration
	Embedded bool
	// Location is a human-readable value that records the location of the handlers configuration.
	Location string
	// Bytes contains the handlers configuration for the application as a slice of bytes.
	Bytes []byte
	// Json contains the handlers configuration for the application in JSON
	Json map[string]interface{}
}

type Configuration struct {
	*Config
	Key string
}

type Configurable interface {
	Configure(c Configuration) error
}

// Resolve determines the location of the application configuration file and unmarshals it into JSON.  It records the
// location and its contents in Config.
//
// cliValue may be the empty string.  If cliValue is provided, it is used as-is to reference a path on the filesystem.
// If empty, then the environment variable `DERIVATIVE_HANDLER_CONFIG` is consulted.  If no value is provided for the
// `DERIVATIVE_HANDLER_CONFIG` variable, then the embedded configuration is used.
//
// This method will terminate the application if a configuration cannot be found and successfully parsed.
func (s *Config) Resolve(cliValue string) {
	var err error
	if cliValue == "" {
		if s.Location = env.GetOrDefault(VarHandlerConfig, ""); s.Location == "" {
			if s.Bytes, err = configFs.ReadFile(defaultConfig); err != nil {
				log.Fatalf("config: error reading embedded configuration '%s': %s", defaultConfig, err)
			} else {
				s.Embedded = true
				s.Location = defaultConfig
			}
		} else {
			if s.Bytes, err = os.ReadFile(s.Location); err != nil {
				log.Fatalf("config: error reading configuration from '%s': %s", s.Location, err)
			}
		}
	} else {
		s.Location = cliValue
		if s.Bytes, err = os.ReadFile(s.Location); err != nil {
			log.Fatalf("config: error reading configuration from '%s': %s", s.Location, err)
		}
	}

	if err = json.Unmarshal(s.Bytes, &s.Json); err != nil {
		log.Fatalf("config: error unmarshaling configuration from '%s': '%s", err)
	}
}

// StringValue returns a portion of the application Config as a string.
//
// The jsonBlob represents all or a portion of the application configuration expected to contain the provided top-level
// key.  The result is the value represented by the key as a string.
//
// If the key is not found, a NotFoundErr will be returned.  If the keyed value cannot be converted to a string, a
// TypeConvErr is returned.
func (s *Config) StringValue(jsonBlob *map[string]interface{}, key string) (string, error) {
	if _, ok := (*jsonBlob)[key]; !ok {
		return "", notFoundErr(key)
	}

	if _, ok := (*jsonBlob)[key].(string); !ok {
		return "", typeConversionErr(key, (*jsonBlob)[key], "")
	}

	return (*jsonBlob)[key].(string), nil
}

// SliceStringValue returns a portion of the application Config as a []string.
//
// The jsonBlob represents all or a portion of the application configuration expected to contain the provided top-level
// key.  The result is the value represented by the key as a []string.
//
// If the key is not found, a NotFoundErr will be returned.  If the keyed value cannot be converted to a []string, a
// TypeConvErr is returned.
func (s *Config) SliceStringValue(jsonBlob *map[string]interface{}, key string) ([]string, error) {
	var (
		slice  []interface{}
		result []string
		ok     bool
	)

	if _, ok = (*jsonBlob)[key]; !ok {
		return []string{}, notFoundErr(key)
	}

	if slice, ok = (*jsonBlob)[key].([]interface{}); !ok {
		return []string{}, typeConversionErr(key, (*jsonBlob)[key], []interface{}{})
	}

	for _, element := range slice {
		if value, ok := element.(string); !ok {
			return []string{}, typeConversionErr(key, (*jsonBlob)[key], "")
		} else {
			result = append(result, value)
		}
	}

	return result, nil
}

// BoolValue returns a portion of the application Config as a bool.
//
// The jsonBlob represents all or a portion of the application configuration expected to contain the provided top-level
// key.  The result is the value represented by the key as a bool.
//
// If the key is not found, a NotFoundErr will be returned.  If the keyed value cannot be converted to a bool, a
// TypeConvErr is returned.
func (s *Config) BoolValue(jsonBlob *map[string]interface{}, key string) (bool, error) {
	if _, ok := (*jsonBlob)[key]; !ok {
		return false, notFoundErr(key)
	}

	if _, ok := (*jsonBlob)[key].(bool); !ok {
		return false, typeConversionErr(key, (*jsonBlob)[key], false)
	}

	return (*jsonBlob)[key].(bool), nil
}

// MapValue returns a portion of the application Config as a map[string]interface{}.
//
// The jsonBlob represents all or a portion of the application configuration expected to contain the provided top-level
// key.  The result is the value represented by the key as a map[string]interface{}.
//
// If the key is not found, a NotFoundErr will be returned.  If the keyed value cannot be converted to a
// map[string]interface{}, a TypeConvErr is returned.
func (s *Config) MapValue(jsonBlob *map[string]interface{}, key string) (map[string]interface{}, error) {
	if _, ok := (*jsonBlob)[key]; !ok {
		return map[string]interface{}{}, notFoundErr(key)
	}

	if _, ok := (*jsonBlob)[key].(map[string]interface{}); !ok {
		return map[string]interface{}{}, typeConversionErr(key, (*jsonBlob)[key], map[string]interface{}{})
	}

	return (*jsonBlob)[key].(map[string]interface{}), nil
}

// UnmarshalHandlerConfig unmarshals a Handler configuration from Config, based on the Configuration.Key.  If the key
// is missing, or the configuration cannot be parsed, an error is returned.
func (c *Configuration) UnmarshalHandlerConfig() (*map[string]interface{}, error) {
	var err error
	var handlerConfig map[string]interface{}

	if handlerConfig, err = c.MapValue(&c.Json, c.Key); err != nil {
		return nil, fmt.Errorf("config: unable to configure handler with key '%s': %w", c.Key, err)
	}
	return &handlerConfig, nil
}

// Answers an error such that errors.Is(err, NotFoundErr) == true
func notFoundErr(key string) error {
	return fmt.Errorf("config: %w: '%s'", NotFoundErr, key)
}

// Answers an error such that errors.Is(err, TypeConvErr) == true.  srcType and targetType are *instances* of the source
// and target types.
func typeConversionErr(key string, srcType, targetType interface{}) error {
	return fmt.Errorf("config: %w: unable to convert value for configuration key '%s' from %T to %T", TypeConvErr, key, srcType, targetType)
}
