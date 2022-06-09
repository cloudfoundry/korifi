package util

import (
	"code.cloudfoundry.org/lager"
	"github.com/go-logr/logr"
)

// LagerLogr is a logr (https://github.com/go-logr/logr) implementation over lager.Logger
type LagerLogr struct {
	logger lager.Logger
}

func NewLagerLogr(logger lager.Logger) logr.Logger {
	return logr.New(LagerLogr{logger: logger})
}

func (l LagerLogr) Enabled(level int) bool {
	return true
}

func (l LagerLogr) Info(level int, msg string, kvs ...interface{}) {
	l.logger.Info(msg, toLagerData(kvs))
}

func (l LagerLogr) Error(err error, msg string, kvs ...interface{}) {
	l.logger.Error(msg, err, toLagerData(kvs))
}

func (l LagerLogr) Init(_ logr.RuntimeInfo) {}

func (l LagerLogr) WithValues(kvs ...interface{}) logr.LogSink {
	l.logger = l.logger.WithData(toLagerData(kvs))

	return l
}

func (l LagerLogr) WithName(name string) logr.LogSink {
	l.logger = l.logger.Session(name)

	return l
}

func toLagerData(kvs []interface{}) lager.Data {
	data := lager.Data{}
	for i := 0; i < len(kvs); i += 2 {
		data[kvs[i].(string)] = kvs[i+1] //nolint:forcetypeassert
	}

	return data
}
