package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerctx"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/gbytes"
)

// This is a copy of lagger's test logger. The original uses ginkgo1 which is not compatible with ginkgo2 tests. So we just copied it and made it use ginkgo2
// See also https://github.com/onsi/ginkgo/issues/875

type TestLogger struct {
	lager.Logger
	*TestSink
}

type TestSink struct {
	writeLock *sync.Mutex
	lager.Sink
	buffer *gbytes.Buffer
	Errors []error
}

func NewTestLogger(component string) *TestLogger {
	logger := lager.NewLogger(component)

	testSink := NewTestSink()
	logger.RegisterSink(testSink)
	logger.RegisterSink(lager.NewWriterSink(ginkgo.GinkgoWriter, lager.DEBUG))

	return &TestLogger{logger, testSink}
}

func NewContext(parent context.Context, name string) context.Context {
	return lagerctx.NewContext(parent, NewTestLogger(name))
}

func NewTestSink() *TestSink {
	buffer := gbytes.NewBuffer()

	return &TestSink{
		writeLock: new(sync.Mutex),
		Sink:      lager.NewWriterSink(buffer, lager.DEBUG),
		buffer:    buffer,
	}
}

func (s *TestSink) Buffer() *gbytes.Buffer {
	return s.buffer
}

func (s *TestSink) Logs() []lager.LogFormat {
	logs := []lager.LogFormat{}
	decoder := json.NewDecoder(bytes.NewBuffer(s.buffer.Contents()))

	for {
		var log lager.LogFormat
		if err := decoder.Decode(&log); errors.Is(err, io.EOF) {
			return logs
		} else if err != nil {
			panic(err)
		}

		logs = append(logs, log)
	}
}

func (s *TestSink) LogMessages() []string {
	logs := s.Logs()
	messages := make([]string, 0, len(logs))

	for _, log := range logs {
		messages = append(messages, log.Message)
	}

	return messages
}

func (s *TestSink) Log(log lager.LogFormat) {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	if log.Error != nil {
		s.Errors = append(s.Errors, log.Error)
	}

	s.Sink.Log(log)
}
