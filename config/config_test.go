package config

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

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
