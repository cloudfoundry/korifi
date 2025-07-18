// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
)

type BrokerClientFactory struct {
	CreateClientStub        func(context.Context, *v1alpha1.CFServiceBroker) (osbapi.BrokerClient, error)
	createClientMutex       sync.RWMutex
	createClientArgsForCall []struct {
		arg1 context.Context
		arg2 *v1alpha1.CFServiceBroker
	}
	createClientReturns struct {
		result1 osbapi.BrokerClient
		result2 error
	}
	createClientReturnsOnCall map[int]struct {
		result1 osbapi.BrokerClient
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *BrokerClientFactory) CreateClient(arg1 context.Context, arg2 *v1alpha1.CFServiceBroker) (osbapi.BrokerClient, error) {
	fake.createClientMutex.Lock()
	ret, specificReturn := fake.createClientReturnsOnCall[len(fake.createClientArgsForCall)]
	fake.createClientArgsForCall = append(fake.createClientArgsForCall, struct {
		arg1 context.Context
		arg2 *v1alpha1.CFServiceBroker
	}{arg1, arg2})
	stub := fake.CreateClientStub
	fakeReturns := fake.createClientReturns
	fake.recordInvocation("CreateClient", []interface{}{arg1, arg2})
	fake.createClientMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *BrokerClientFactory) CreateClientCallCount() int {
	fake.createClientMutex.RLock()
	defer fake.createClientMutex.RUnlock()
	return len(fake.createClientArgsForCall)
}

func (fake *BrokerClientFactory) CreateClientCalls(stub func(context.Context, *v1alpha1.CFServiceBroker) (osbapi.BrokerClient, error)) {
	fake.createClientMutex.Lock()
	defer fake.createClientMutex.Unlock()
	fake.CreateClientStub = stub
}

func (fake *BrokerClientFactory) CreateClientArgsForCall(i int) (context.Context, *v1alpha1.CFServiceBroker) {
	fake.createClientMutex.RLock()
	defer fake.createClientMutex.RUnlock()
	argsForCall := fake.createClientArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *BrokerClientFactory) CreateClientReturns(result1 osbapi.BrokerClient, result2 error) {
	fake.createClientMutex.Lock()
	defer fake.createClientMutex.Unlock()
	fake.CreateClientStub = nil
	fake.createClientReturns = struct {
		result1 osbapi.BrokerClient
		result2 error
	}{result1, result2}
}

func (fake *BrokerClientFactory) CreateClientReturnsOnCall(i int, result1 osbapi.BrokerClient, result2 error) {
	fake.createClientMutex.Lock()
	defer fake.createClientMutex.Unlock()
	fake.CreateClientStub = nil
	if fake.createClientReturnsOnCall == nil {
		fake.createClientReturnsOnCall = make(map[int]struct {
			result1 osbapi.BrokerClient
			result2 error
		})
	}
	fake.createClientReturnsOnCall[i] = struct {
		result1 osbapi.BrokerClient
		result2 error
	}{result1, result2}
}

func (fake *BrokerClientFactory) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *BrokerClientFactory) recordInvocation(key string, args []interface{}) {
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

var _ osbapi.BrokerClientFactory = new(BrokerClientFactory)
