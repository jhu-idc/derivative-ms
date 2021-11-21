package listener

import "fmt"

type StompLogger struct{}

func (StompLogger) Debugf(format string, value ...interface{}) {
	fmt.Printf(format, value)
}

func (StompLogger) Infof(format string, value ...interface{}) {
	fmt.Printf(format, value)
}

func (StompLogger) Warningf(format string, value ...interface{}) {
	fmt.Printf(format, value)
}

func (StompLogger) Errorf(format string, value ...interface{}) {
	fmt.Printf(format, value)
}

func (StompLogger) Debug(message string) {
	fmt.Print(message)
}

func (StompLogger) Info(message string) {
	fmt.Print(message)
}

func (StompLogger) Warning(message string) {
	fmt.Print(message)
}

func (StompLogger) Error(message string) {
	fmt.Print(message)
}
