// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
)

type SpaceRepository struct {
	ListSpacesStub        func(context.Context, authorization.Info, repositories.ListSpacesMessage) (repositories.ListResult[repositories.SpaceRecord], error)
	listSpacesMutex       sync.RWMutex
	listSpacesArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListSpacesMessage
	}
	listSpacesReturns struct {
		result1 repositories.ListResult[repositories.SpaceRecord]
		result2 error
	}
	listSpacesReturnsOnCall map[int]struct {
		result1 repositories.ListResult[repositories.SpaceRecord]
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *SpaceRepository) ListSpaces(arg1 context.Context, arg2 authorization.Info, arg3 repositories.ListSpacesMessage) (repositories.ListResult[repositories.SpaceRecord], error) {
	fake.listSpacesMutex.Lock()
	ret, specificReturn := fake.listSpacesReturnsOnCall[len(fake.listSpacesArgsForCall)]
	fake.listSpacesArgsForCall = append(fake.listSpacesArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListSpacesMessage
	}{arg1, arg2, arg3})
	stub := fake.ListSpacesStub
	fakeReturns := fake.listSpacesReturns
	fake.recordInvocation("ListSpaces", []interface{}{arg1, arg2, arg3})
	fake.listSpacesMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *SpaceRepository) ListSpacesCallCount() int {
	fake.listSpacesMutex.RLock()
	defer fake.listSpacesMutex.RUnlock()
	return len(fake.listSpacesArgsForCall)
}

func (fake *SpaceRepository) ListSpacesCalls(stub func(context.Context, authorization.Info, repositories.ListSpacesMessage) (repositories.ListResult[repositories.SpaceRecord], error)) {
	fake.listSpacesMutex.Lock()
	defer fake.listSpacesMutex.Unlock()
	fake.ListSpacesStub = stub
}

func (fake *SpaceRepository) ListSpacesArgsForCall(i int) (context.Context, authorization.Info, repositories.ListSpacesMessage) {
	fake.listSpacesMutex.RLock()
	defer fake.listSpacesMutex.RUnlock()
	argsForCall := fake.listSpacesArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *SpaceRepository) ListSpacesReturns(result1 repositories.ListResult[repositories.SpaceRecord], result2 error) {
	fake.listSpacesMutex.Lock()
	defer fake.listSpacesMutex.Unlock()
	fake.ListSpacesStub = nil
	fake.listSpacesReturns = struct {
		result1 repositories.ListResult[repositories.SpaceRecord]
		result2 error
	}{result1, result2}
}

func (fake *SpaceRepository) ListSpacesReturnsOnCall(i int, result1 repositories.ListResult[repositories.SpaceRecord], result2 error) {
	fake.listSpacesMutex.Lock()
	defer fake.listSpacesMutex.Unlock()
	fake.ListSpacesStub = nil
	if fake.listSpacesReturnsOnCall == nil {
		fake.listSpacesReturnsOnCall = make(map[int]struct {
			result1 repositories.ListResult[repositories.SpaceRecord]
			result2 error
		})
	}
	fake.listSpacesReturnsOnCall[i] = struct {
		result1 repositories.ListResult[repositories.SpaceRecord]
		result2 error
	}{result1, result2}
}

func (fake *SpaceRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *SpaceRepository) recordInvocation(key string, args []interface{}) {
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

var _ relationships.SpaceRepository = new(SpaceRepository)
