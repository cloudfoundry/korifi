// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/stats"
)

type GaugesCollector struct {
	CollectProcessGaugesStub        func(context.Context, string, string) ([]stats.ProcessGauges, error)
	collectProcessGaugesMutex       sync.RWMutex
	collectProcessGaugesArgsForCall []struct {
		arg1 context.Context
		arg2 string
		arg3 string
	}
	collectProcessGaugesReturns struct {
		result1 []stats.ProcessGauges
		result2 error
	}
	collectProcessGaugesReturnsOnCall map[int]struct {
		result1 []stats.ProcessGauges
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *GaugesCollector) CollectProcessGauges(arg1 context.Context, arg2 string, arg3 string) ([]stats.ProcessGauges, error) {
	fake.collectProcessGaugesMutex.Lock()
	ret, specificReturn := fake.collectProcessGaugesReturnsOnCall[len(fake.collectProcessGaugesArgsForCall)]
	fake.collectProcessGaugesArgsForCall = append(fake.collectProcessGaugesArgsForCall, struct {
		arg1 context.Context
		arg2 string
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.CollectProcessGaugesStub
	fakeReturns := fake.collectProcessGaugesReturns
	fake.recordInvocation("CollectProcessGauges", []interface{}{arg1, arg2, arg3})
	fake.collectProcessGaugesMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *GaugesCollector) CollectProcessGaugesCallCount() int {
	fake.collectProcessGaugesMutex.RLock()
	defer fake.collectProcessGaugesMutex.RUnlock()
	return len(fake.collectProcessGaugesArgsForCall)
}

func (fake *GaugesCollector) CollectProcessGaugesCalls(stub func(context.Context, string, string) ([]stats.ProcessGauges, error)) {
	fake.collectProcessGaugesMutex.Lock()
	defer fake.collectProcessGaugesMutex.Unlock()
	fake.CollectProcessGaugesStub = stub
}

func (fake *GaugesCollector) CollectProcessGaugesArgsForCall(i int) (context.Context, string, string) {
	fake.collectProcessGaugesMutex.RLock()
	defer fake.collectProcessGaugesMutex.RUnlock()
	argsForCall := fake.collectProcessGaugesArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *GaugesCollector) CollectProcessGaugesReturns(result1 []stats.ProcessGauges, result2 error) {
	fake.collectProcessGaugesMutex.Lock()
	defer fake.collectProcessGaugesMutex.Unlock()
	fake.CollectProcessGaugesStub = nil
	fake.collectProcessGaugesReturns = struct {
		result1 []stats.ProcessGauges
		result2 error
	}{result1, result2}
}

func (fake *GaugesCollector) CollectProcessGaugesReturnsOnCall(i int, result1 []stats.ProcessGauges, result2 error) {
	fake.collectProcessGaugesMutex.Lock()
	defer fake.collectProcessGaugesMutex.Unlock()
	fake.CollectProcessGaugesStub = nil
	if fake.collectProcessGaugesReturnsOnCall == nil {
		fake.collectProcessGaugesReturnsOnCall = make(map[int]struct {
			result1 []stats.ProcessGauges
			result2 error
		})
	}
	fake.collectProcessGaugesReturnsOnCall[i] = struct {
		result1 []stats.ProcessGauges
		result2 error
	}{result1, result2}
}

func (fake *GaugesCollector) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *GaugesCollector) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ handlers.GaugesCollector = new(GaugesCollector)
