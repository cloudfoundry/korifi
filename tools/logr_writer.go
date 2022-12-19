package tools

import (
	"errors"

	"github.com/go-logr/logr"
)

// LogrWriter implements io.Writer and converts Write calls to logr.Logger.Error() calls
type LogrWriter struct {
	Logger  logr.Logger
	Message string
}

func (w *LogrWriter) Write(msg []byte) (int, error) {
	w.Logger.Error(errors.New(string(msg)), w.Message)
	return len(msg), nil
}
