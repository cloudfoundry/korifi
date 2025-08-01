// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/actions/shared"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type CFDomainRepository struct {
	ListDomainsStub        func(context.Context, authorization.Info, repositories.ListDomainsMessage) (repositories.ListResult[repositories.DomainRecord], error)
	listDomainsMutex       sync.RWMutex
	listDomainsArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListDomainsMessage
	}
	listDomainsReturns struct {
		result1 repositories.ListResult[repositories.DomainRecord]
		result2 error
	}
	listDomainsReturnsOnCall map[int]struct {
		result1 repositories.ListResult[repositories.DomainRecord]
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *CFDomainRepository) ListDomains(arg1 context.Context, arg2 authorization.Info, arg3 repositories.ListDomainsMessage) (repositories.ListResult[repositories.DomainRecord], error) {
	fake.listDomainsMutex.Lock()
	ret, specificReturn := fake.listDomainsReturnsOnCall[len(fake.listDomainsArgsForCall)]
	fake.listDomainsArgsForCall = append(fake.listDomainsArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListDomainsMessage
	}{arg1, arg2, arg3})
	stub := fake.ListDomainsStub
	fakeReturns := fake.listDomainsReturns
	fake.recordInvocation("ListDomains", []interface{}{arg1, arg2, arg3})
	fake.listDomainsMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *CFDomainRepository) ListDomainsCallCount() int {
	fake.listDomainsMutex.RLock()
	defer fake.listDomainsMutex.RUnlock()
	return len(fake.listDomainsArgsForCall)
}

func (fake *CFDomainRepository) ListDomainsCalls(stub func(context.Context, authorization.Info, repositories.ListDomainsMessage) (repositories.ListResult[repositories.DomainRecord], error)) {
	fake.listDomainsMutex.Lock()
	defer fake.listDomainsMutex.Unlock()
	fake.ListDomainsStub = stub
}

func (fake *CFDomainRepository) ListDomainsArgsForCall(i int) (context.Context, authorization.Info, repositories.ListDomainsMessage) {
	fake.listDomainsMutex.RLock()
	defer fake.listDomainsMutex.RUnlock()
	argsForCall := fake.listDomainsArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *CFDomainRepository) ListDomainsReturns(result1 repositories.ListResult[repositories.DomainRecord], result2 error) {
	fake.listDomainsMutex.Lock()
	defer fake.listDomainsMutex.Unlock()
	fake.ListDomainsStub = nil
	fake.listDomainsReturns = struct {
		result1 repositories.ListResult[repositories.DomainRecord]
		result2 error
	}{result1, result2}
}

func (fake *CFDomainRepository) ListDomainsReturnsOnCall(i int, result1 repositories.ListResult[repositories.DomainRecord], result2 error) {
	fake.listDomainsMutex.Lock()
	defer fake.listDomainsMutex.Unlock()
	fake.ListDomainsStub = nil
	if fake.listDomainsReturnsOnCall == nil {
		fake.listDomainsReturnsOnCall = make(map[int]struct {
			result1 repositories.ListResult[repositories.DomainRecord]
			result2 error
		})
	}
	fake.listDomainsReturnsOnCall[i] = struct {
		result1 repositories.ListResult[repositories.DomainRecord]
		result2 error
	}{result1, result2}
}

func (fake *CFDomainRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *CFDomainRepository) recordInvocation(key string, args []interface{}) {
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

var _ shared.CFDomainRepository = new(CFDomainRepository)
