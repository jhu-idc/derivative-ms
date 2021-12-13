package listener

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SetListenerDefaultsTT(t *testing.T) {
	type setter func(l *StompListener)

	tests := []struct {
		name      string
		underTest StompListener
		setup     []setter
		expected  StompListener
	}{
		{
			"Default Values",
			StompListener{},
			[]setter{},
			StompListener{
				Host:         defaultHost,
				Port:         defaultPort,
				User:         defaultUser,
				Pass:         "",
				Queue:        "",
				AckMode:      defaultAckMode,
				StompHandler: nil,
			},
		},
		{
			"Set Host",
			StompListener{},
			[]setter{func(l *StompListener) {
				l.Host = "moo"
			}},
			StompListener{
				Host:         "moo",
				Port:         defaultPort,
				User:         defaultUser,
				Pass:         "",
				Queue:        "",
				AckMode:      defaultAckMode,
				StompHandler: nil,
			},
		},
		{
			"Set Port",
			StompListener{},
			[]setter{func(l *StompListener) {
				l.Port = 2020
			}},
			StompListener{
				Host:         defaultHost,
				Port:         2020,
				User:         defaultUser,
				Pass:         "",
				Queue:        "",
				AckMode:      defaultAckMode,
				StompHandler: nil,
			},
		},
		{
			"Set Ack Mode",
			StompListener{},
			[]setter{func(l *StompListener) {
				l.AckMode = "foo"
			}},
			StompListener{
				Host:         defaultHost,
				Port:         defaultPort,
				User:         defaultUser,
				Pass:         "",
				Queue:        "",
				AckMode:      "foo",
				StompHandler: nil,
			},
		},
		{
			"Set User",
			StompListener{},
			[]setter{func(l *StompListener) {
				l.User = "moo"
			}},
			StompListener{
				Host:         defaultHost,
				Port:         defaultPort,
				User:         "moo",
				Pass:         "",
				Queue:        "",
				AckMode:      defaultAckMode,
				StompHandler: nil,
			},
		},
		{
			"Set Pass",
			StompListener{},
			[]setter{func(l *StompListener) {
				l.Pass = "foo"
			}},
			StompListener{
				Host:         defaultHost,
				Port:         defaultPort,
				User:         defaultUser,
				Pass:         "foo",
				Queue:        "",
				AckMode:      defaultAckMode,
				StompHandler: nil,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// setup state of the StompListener
			for _, s := range test.setup {
				s(&test.underTest)
			}

			// apply the listener defaults, which should only be set if the listener defaults are their zero value
			setListenerDefaults(&test.underTest)

			// verify expectations
			assert.Equal(t, test.expected.Host, test.underTest.Host)
			assert.Equal(t, test.expected.Port, test.underTest.Port)
			assert.Equal(t, test.expected.AckMode, test.underTest.AckMode)
			assert.Equal(t, test.expected.User, test.underTest.User)
			assert.Equal(t, test.expected.Pass, test.underTest.Pass)
			assert.Equal(t, test.expected.StompHandler, test.underTest.StompHandler)
		})
	}
}
