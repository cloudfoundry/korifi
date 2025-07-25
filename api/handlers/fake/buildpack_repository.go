// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type BuildpackRepository struct {
	ListBuildpacksStub        func(context.Context, authorization.Info, repositories.ListBuildpacksMessage) (repositories.ListResult[repositories.BuildpackRecord], error)
	listBuildpacksMutex       sync.RWMutex
	listBuildpacksArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListBuildpacksMessage
	}
	listBuildpacksReturns struct {
		result1 repositories.ListResult[repositories.BuildpackRecord]
		result2 error
	}
	listBuildpacksReturnsOnCall map[int]struct {
		result1 repositories.ListResult[repositories.BuildpackRecord]
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *BuildpackRepository) ListBuildpacks(arg1 context.Context, arg2 authorization.Info, arg3 repositories.ListBuildpacksMessage) (repositories.ListResult[repositories.BuildpackRecord], error) {
	fake.listBuildpacksMutex.Lock()
	ret, specificReturn := fake.listBuildpacksReturnsOnCall[len(fake.listBuildpacksArgsForCall)]
	fake.listBuildpacksArgsForCall = append(fake.listBuildpacksArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListBuildpacksMessage
	}{arg1, arg2, arg3})
	stub := fake.ListBuildpacksStub
	fakeReturns := fake.listBuildpacksReturns
	fake.recordInvocation("ListBuildpacks", []interface{}{arg1, arg2, arg3})
	fake.listBuildpacksMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *BuildpackRepository) ListBuildpacksCallCount() int {
	fake.listBuildpacksMutex.RLock()
	defer fake.listBuildpacksMutex.RUnlock()
	return len(fake.listBuildpacksArgsForCall)
}

func (fake *BuildpackRepository) ListBuildpacksCalls(stub func(context.Context, authorization.Info, repositories.ListBuildpacksMessage) (repositories.ListResult[repositories.BuildpackRecord], error)) {
	fake.listBuildpacksMutex.Lock()
	defer fake.listBuildpacksMutex.Unlock()
	fake.ListBuildpacksStub = stub
}

func (fake *BuildpackRepository) ListBuildpacksArgsForCall(i int) (context.Context, authorization.Info, repositories.ListBuildpacksMessage) {
	fake.listBuildpacksMutex.RLock()
	defer fake.listBuildpacksMutex.RUnlock()
	argsForCall := fake.listBuildpacksArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *BuildpackRepository) ListBuildpacksReturns(result1 repositories.ListResult[repositories.BuildpackRecord], result2 error) {
	fake.listBuildpacksMutex.Lock()
	defer fake.listBuildpacksMutex.Unlock()
	fake.ListBuildpacksStub = nil
	fake.listBuildpacksReturns = struct {
		result1 repositories.ListResult[repositories.BuildpackRecord]
		result2 error
	}{result1, result2}
}

func (fake *BuildpackRepository) ListBuildpacksReturnsOnCall(i int, result1 repositories.ListResult[repositories.BuildpackRecord], result2 error) {
	fake.listBuildpacksMutex.Lock()
	defer fake.listBuildpacksMutex.Unlock()
	fake.ListBuildpacksStub = nil
	if fake.listBuildpacksReturnsOnCall == nil {
		fake.listBuildpacksReturnsOnCall = make(map[int]struct {
			result1 repositories.ListResult[repositories.BuildpackRecord]
			result2 error
		})
	}
	fake.listBuildpacksReturnsOnCall[i] = struct {
		result1 repositories.ListResult[repositories.BuildpackRecord]
		result2 error
	}{result1, result2}
}

func (fake *BuildpackRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *BuildpackRepository) recordInvocation(key string, args []interface{}) {
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

var _ handlers.BuildpackRepository = new(BuildpackRepository)
