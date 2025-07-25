// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
)

type ServiceBrokerRepository struct {
	ListServiceBrokersStub        func(context.Context, authorization.Info, repositories.ListServiceBrokerMessage) (repositories.ListResult[repositories.ServiceBrokerRecord], error)
	listServiceBrokersMutex       sync.RWMutex
	listServiceBrokersArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListServiceBrokerMessage
	}
	listServiceBrokersReturns struct {
		result1 repositories.ListResult[repositories.ServiceBrokerRecord]
		result2 error
	}
	listServiceBrokersReturnsOnCall map[int]struct {
		result1 repositories.ListResult[repositories.ServiceBrokerRecord]
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *ServiceBrokerRepository) ListServiceBrokers(arg1 context.Context, arg2 authorization.Info, arg3 repositories.ListServiceBrokerMessage) (repositories.ListResult[repositories.ServiceBrokerRecord], error) {
	fake.listServiceBrokersMutex.Lock()
	ret, specificReturn := fake.listServiceBrokersReturnsOnCall[len(fake.listServiceBrokersArgsForCall)]
	fake.listServiceBrokersArgsForCall = append(fake.listServiceBrokersArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListServiceBrokerMessage
	}{arg1, arg2, arg3})
	stub := fake.ListServiceBrokersStub
	fakeReturns := fake.listServiceBrokersReturns
	fake.recordInvocation("ListServiceBrokers", []interface{}{arg1, arg2, arg3})
	fake.listServiceBrokersMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *ServiceBrokerRepository) ListServiceBrokersCallCount() int {
	fake.listServiceBrokersMutex.RLock()
	defer fake.listServiceBrokersMutex.RUnlock()
	return len(fake.listServiceBrokersArgsForCall)
}

func (fake *ServiceBrokerRepository) ListServiceBrokersCalls(stub func(context.Context, authorization.Info, repositories.ListServiceBrokerMessage) (repositories.ListResult[repositories.ServiceBrokerRecord], error)) {
	fake.listServiceBrokersMutex.Lock()
	defer fake.listServiceBrokersMutex.Unlock()
	fake.ListServiceBrokersStub = stub
}

func (fake *ServiceBrokerRepository) ListServiceBrokersArgsForCall(i int) (context.Context, authorization.Info, repositories.ListServiceBrokerMessage) {
	fake.listServiceBrokersMutex.RLock()
	defer fake.listServiceBrokersMutex.RUnlock()
	argsForCall := fake.listServiceBrokersArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *ServiceBrokerRepository) ListServiceBrokersReturns(result1 repositories.ListResult[repositories.ServiceBrokerRecord], result2 error) {
	fake.listServiceBrokersMutex.Lock()
	defer fake.listServiceBrokersMutex.Unlock()
	fake.ListServiceBrokersStub = nil
	fake.listServiceBrokersReturns = struct {
		result1 repositories.ListResult[repositories.ServiceBrokerRecord]
		result2 error
	}{result1, result2}
}

func (fake *ServiceBrokerRepository) ListServiceBrokersReturnsOnCall(i int, result1 repositories.ListResult[repositories.ServiceBrokerRecord], result2 error) {
	fake.listServiceBrokersMutex.Lock()
	defer fake.listServiceBrokersMutex.Unlock()
	fake.ListServiceBrokersStub = nil
	if fake.listServiceBrokersReturnsOnCall == nil {
		fake.listServiceBrokersReturnsOnCall = make(map[int]struct {
			result1 repositories.ListResult[repositories.ServiceBrokerRecord]
			result2 error
		})
	}
	fake.listServiceBrokersReturnsOnCall[i] = struct {
		result1 repositories.ListResult[repositories.ServiceBrokerRecord]
		result2 error
	}{result1, result2}
}

func (fake *ServiceBrokerRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *ServiceBrokerRepository) recordInvocation(key string, args []interface{}) {
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

var _ relationships.ServiceBrokerRepository = new(ServiceBrokerRepository)
