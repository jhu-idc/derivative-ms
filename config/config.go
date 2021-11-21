package config

import (
	"derivative-ms/env"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

const (
	defaultConfig    = "handlers.json"
	VarHandlerConfig = "DERIVATIVE_HANDLER_CONFIG"
)

//go:embed *.json
var configFs embed.FS

type Config struct {
	Embedded bool
	Location string
	Bytes    []byte
	Json     map[string]interface{}
}

type Configuration struct {
	*Config
	Key string
}

type Configurable interface {
	Configure() error
}

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

func StringValue(jsonBlob *map[string]interface{}, key string) (string, error) {
	if _, ok := (*jsonBlob)[key]; !ok {
		return "", fmt.Errorf("config: missing value for '%s'", key)
	}

	if _, ok := (*jsonBlob)[key].(string); !ok {
		return "", fmt.Errorf("config: unable to convert value for '%s' from %T to %T", key, (*jsonBlob)[key], "")
	}

	return (*jsonBlob)[key].(string), nil
}

func SliceStringValue(jsonBlob *map[string]interface{}, key string) ([]string, error) {
	var (
		slice  []interface{}
		result []string
		ok     bool
	)

	if _, ok = (*jsonBlob)[key]; !ok {
		return []string{}, fmt.Errorf("config: missing value for '%s'", key)
	}

	if slice, ok = (*jsonBlob)[key].([]interface{}); !ok {
		return []string{}, fmt.Errorf("config: unable to convert value for '%s' from %T to %T", key, (*jsonBlob)[key], []interface{}{})
	}

	for _, element := range slice {
		if value, ok := element.(string); !ok {
			return []string{}, fmt.Errorf("config: unable to convert value for '%s' from %T to %T", key, (*jsonBlob)[key], []interface{}{})
		} else {
			result = append(result, value)
		}
	}

	return result, nil
}

func BoolValue(jsonBlob *map[string]interface{}, key string) (bool, error) {
	if _, ok := (*jsonBlob)[key]; !ok {
		return false, fmt.Errorf("config: missing value for '%s'", key)
	}

	if _, ok := (*jsonBlob)[key].(bool); !ok {
		return false, fmt.Errorf("config: unable to convert value for '%s' from %T to %T", key, (*jsonBlob)[key], "")
	}

	return (*jsonBlob)[key].(bool), nil
}

func MapValue(jsonBlob *map[string]interface{}, key string) (map[string]interface{}, error) {
	if _, ok := (*jsonBlob)[key]; !ok {
		return map[string]interface{}{}, fmt.Errorf("config: missing value for '%s'", key)
	}

	if _, ok := (*jsonBlob)[key].(map[string]interface{}); !ok {
		return map[string]interface{}{}, fmt.Errorf("config: unable to convert value for '%s' from %T to %T", key, (*jsonBlob)[key], map[string]interface{}{})
	}

	return (*jsonBlob)[key].(map[string]interface{}), nil
}

func (c *Configuration) UnmarshalConfig() (*map[string]interface{}, error) {
	var err error
	var handlerConfig map[string]interface{}

	if handlerConfig, err = MapValue(&c.Json, c.Key); err != nil {
		return nil, fmt.Errorf("config: unable to configure handler with key '%s': %w", c.Key, err)
	}
	return &handlerConfig, nil
}
