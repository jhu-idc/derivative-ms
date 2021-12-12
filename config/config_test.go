package config

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

const (
	// minimal configuration used for testing
	miniConfig = `
{
  "jwt-logger": {
    "handler-type": "JWTLoggingHandler",
	"order": 10
  }
}
`
)

// testConfig is a minimal handler configuration, represented as a string
type testConfig string

// verify ensures the provided Config struct represents the content in the testConfig
func (testConfig) verify(t *testing.T, config *Config) {
	expectedKey := "jwt-logger"
	expectedType := "JWTLoggingHandler"
	expectedOrder := 10.0

	var handlerConfig map[string]interface{}

	handlerConfig, ok := config.Json[expectedKey].(map[string]interface{})
	assert.True(t, ok, "the content of the resolved configuration is missing expected handler key '"+expectedKey+"'")
	assert.Equal(t, expectedType, handlerConfig[keyHandlerType], "the resolve handler configuration has an unexpected value for configuration key '"+keyHandlerType+"'")
	assert.Equal(t, expectedOrder, handlerConfig[keyOrder], "the resolve handler configuration has an unexpected value for configuration key '"+keyOrder+"'")
}

func (tc testConfig) asTempFile(fileNamePattern string) *os.File {
	configFile, _ := os.CreateTemp("", fileNamePattern)
	configFile.WriteString(string(tc))
	return configFile
}

func Test_Order(t *testing.T) {
	configs := []Configuration{
		{Key: "moo", Order: 1},
		{Key: "foo", Order: -1},
	}

	Order(configs)

	assert.Equal(t, -1, configs[0].Order)
	assert.Equal(t, "foo", configs[0].Key)

	assert.Equal(t, 1, configs[1].Order)
	assert.Equal(t, "moo", configs[1].Key)
}

func Test_SparseOrder(t *testing.T) {
	configs := []Configuration{
		{Order: 100},
		{Order: 10},
		{Order: 20},
	}

	Order(configs)

	assert.Equal(t, 10, configs[0].Order)
	assert.Equal(t, 20, configs[1].Order)
	assert.Equal(t, 100, configs[2].Order)
}

func Test_ZeroValueOrder(t *testing.T) {
	configs := []Configuration{
		{Key: "moo", Order: 100},
		{Key: "foo"},
		{Key: "bar", Order: 2},
	}

	Order(configs)

	assert.Equal(t, "foo", configs[0].Key)
	assert.Equal(t, "bar", configs[1].Key)
	assert.Equal(t, "moo", configs[2].Key)
}

func Test_ResolveFromCli(t *testing.T) {
	// ensure that the environment var `DERIVATIVE_HANDLER_CONFIG` is not set, and reset it after
	if value, exists := os.LookupEnv(VarHandlerConfig); exists {
		os.Unsetenv(VarHandlerConfig)
		defer os.Setenv(VarHandlerConfig, value)
	}

	configFile := testConfig(miniConfig).asTempFile("Test_ResolveFromCli-config.json")
	defer os.Remove(configFile.Name())
	cliValue := configFile.Name()

	underTest := Config{}
	underTest.Resolve(cliValue)

	assert.False(t, underTest.Embedded, "configuration is found on the filesystem; expected embedded to be false")
	assert.Equal(t, cliValue, underTest.Location, "expected location of configuration to be equal to '"+cliValue+"'")
	assert.Equal(t, []byte(miniConfig), underTest.Bytes, "the content of the resolve configuration is not equal to the content of the provided configuration")

	testConfig(miniConfig).verify(t, &underTest)
}

func Test_ResolveEmbedded(t *testing.T) {
	// ensure that the environment var `DERIVATIVE_HANDLER_CONFIG` is not set, and reset it after
	if value, exists := os.LookupEnv(VarHandlerConfig); exists {
		os.Unsetenv(VarHandlerConfig)
		defer os.Setenv(VarHandlerConfig, value)
	}

	// embedded config
	expectedContent, _ := configFs.ReadFile(defaultConfig)

	underTest := Config{}
	underTest.Resolve("")

	assert.True(t, underTest.Embedded, "expected embedded flag to be true when configuration is not found in the environment or specified by the cli")
	assert.Equal(t, defaultConfig, underTest.Location)
	assert.Equal(t, expectedContent, underTest.Bytes)
}

func Test_ResolveFromEnv(t *testing.T) {
	// ensure that the environment var `DERIVATIVE_HANDLER_CONFIG` is set to a location unique for this test, and reset
	// it after
	if value, exists := os.LookupEnv(VarHandlerConfig); exists {
		os.Unsetenv(VarHandlerConfig)
		defer os.Setenv(VarHandlerConfig, value)
	}

	configFile := testConfig(miniConfig).asTempFile("Test_ResolveFromEnv-config.json")
	defer os.Remove(configFile.Name())
	os.Setenv(VarHandlerConfig, configFile.Name())

	underTest := Config{}
	underTest.Resolve("")

	assert.False(t, underTest.Embedded, "configuration is found on the filesystem; expected embedded to be false")
	assert.Equal(t, configFile.Name(), underTest.Location, "expected location of configuration to be equal to '"+configFile.Name()+"'")
	assert.Equal(t, []byte(miniConfig), underTest.Bytes, "the content of the resolve configuration is not equal to the content of the provided configuration")

	testConfig(miniConfig).verify(t, &underTest)
}

func Test_StringValueNotFoundErr(t *testing.T) {
	_, err := StringValue(new(map[string]interface{}), "moo")
	assert.ErrorIs(t, err, NotFoundErr, "expected the non-existent key 'moo' to result in a NotFoundErr")
}

func Test_StringValueTypeConvErr(t *testing.T) {
	_, err := StringValue(&(map[string]interface{}{"order": 10}), keyOrder)
	assert.ErrorIs(t, err, TypeConvErr, "expected the key '"+keyOrder+"' to result in a TypeConvErr")
}

func Test_StringValueOk(t *testing.T) {
	v, err := StringValue(&(map[string]interface{}{"moo": "foo"}), "moo")
	assert.Nil(t, err)
	assert.Equal(t, "foo", v)
}

func Test_BoolValueNotFoundErr(t *testing.T) {
	_, err := BoolValue(new(map[string]interface{}), "moo")
	assert.ErrorIs(t, err, NotFoundErr, "expected the non-existent key 'moo' to result in a NotFoundErr")
}

func Test_BoolValueTypeConvErr(t *testing.T) {
	_, err := BoolValue(&(map[string]interface{}{"moo": "foo"}), "moo")
	assert.ErrorIs(t, err, TypeConvErr, "expected the key 'moo' to result in a TypeConvErr")
}

func Test_BoolValueOk(t *testing.T) {
	v, err := BoolValue(&(map[string]interface{}{"moo": true}), "moo")
	assert.Nil(t, err)
	assert.True(t, v)
}

func Test_MapValueNotFoundErr(t *testing.T) {
	_, err := MapValue(new(map[string]interface{}), "moo")
	assert.ErrorIs(t, err, NotFoundErr, "expected the non-existent key 'moo' to result in a NotFoundErr")
}

func Test_MapValueTypeConvErr(t *testing.T) {
	_, err := MapValue(&(map[string]interface{}{"moo": "foo"}), "moo")
	assert.ErrorIs(t, err, TypeConvErr, "expected the key 'moo' to result in a TypeConvErr")
}

func Test_MapValueOk(t *testing.T) {
	v, err := MapValue(&(map[string]interface{}{"moo": map[string]interface{}{"foo": "bar"}}), "moo")
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, v)
}

func Test_SliceStringNotFoundErr(t *testing.T) {
	_, err := SliceStringValue(new(map[string]interface{}), "moo")
	assert.ErrorIs(t, err, NotFoundErr, "expected the non-existent key 'moo' to result in a NotFoundErr")
}

func Test_SliceStringTypeConvErr(t *testing.T) {
	_, err := SliceStringValue(&(map[string]interface{}{"moo": "foo"}), "moo")
	assert.ErrorIs(t, err, TypeConvErr, "expected the key 'moo' to result in a TypeConvErr")
}

func Test_SliceStringOkWithSliceString(t *testing.T) {
	v, err := SliceStringValue(&(map[string]interface{}{"moo": []string{"foo", "bar"}}), "moo")
	assert.Nil(t, err)
	assert.Equal(t, []string{"foo", "bar"}, v)
}

func Test_SliceStringOkWithSliceInterface(t *testing.T) {
	v, err := SliceStringValue(&(map[string]interface{}{"moo": []interface{}{"foo", "bar"}}), "moo")
	assert.Nil(t, err)
	assert.Equal(t, []string{"foo", "bar"}, v)
}

func Test_SliceStringOkWithSliceInt(t *testing.T) {
	_, err := SliceStringValue(&(map[string]interface{}{"moo": []int{10, 20}}), "moo")
	assert.ErrorIs(t, err, TypeConvErr, "expected the key 'moo' to result in a TypeConvErr")
}
