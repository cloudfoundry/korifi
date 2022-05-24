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
	GetBuildpacksForBuilderStub        func(context.Context, authorization.Info, string) ([]repositories.BuildpackRecord, error)
	getBuildpacksForBuilderMutex       sync.RWMutex
	getBuildpacksForBuilderArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}
	getBuildpacksForBuilderReturns struct {
		result1 []repositories.BuildpackRecord
		result2 error
	}
	getBuildpacksForBuilderReturnsOnCall map[int]struct {
		result1 []repositories.BuildpackRecord
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *BuildpackRepository) GetBuildpacksForBuilder(arg1 context.Context, arg2 authorization.Info, arg3 string) ([]repositories.BuildpackRecord, error) {
	fake.getBuildpacksForBuilderMutex.Lock()
	ret, specificReturn := fake.getBuildpacksForBuilderReturnsOnCall[len(fake.getBuildpacksForBuilderArgsForCall)]
	fake.getBuildpacksForBuilderArgsForCall = append(fake.getBuildpacksForBuilderArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.GetBuildpacksForBuilderStub
	fakeReturns := fake.getBuildpacksForBuilderReturns
	fake.recordInvocation("GetBuildpacksForBuilder", []interface{}{arg1, arg2, arg3})
	fake.getBuildpacksForBuilderMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *BuildpackRepository) GetBuildpacksForBuilderCallCount() int {
	fake.getBuildpacksForBuilderMutex.RLock()
	defer fake.getBuildpacksForBuilderMutex.RUnlock()
	return len(fake.getBuildpacksForBuilderArgsForCall)
}

func (fake *BuildpackRepository) GetBuildpacksForBuilderCalls(stub func(context.Context, authorization.Info, string) ([]repositories.BuildpackRecord, error)) {
	fake.getBuildpacksForBuilderMutex.Lock()
	defer fake.getBuildpacksForBuilderMutex.Unlock()
	fake.GetBuildpacksForBuilderStub = stub
}

func (fake *BuildpackRepository) GetBuildpacksForBuilderArgsForCall(i int) (context.Context, authorization.Info, string) {
	fake.getBuildpacksForBuilderMutex.RLock()
	defer fake.getBuildpacksForBuilderMutex.RUnlock()
	argsForCall := fake.getBuildpacksForBuilderArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *BuildpackRepository) GetBuildpacksForBuilderReturns(result1 []repositories.BuildpackRecord, result2 error) {
	fake.getBuildpacksForBuilderMutex.Lock()
	defer fake.getBuildpacksForBuilderMutex.Unlock()
	fake.GetBuildpacksForBuilderStub = nil
	fake.getBuildpacksForBuilderReturns = struct {
		result1 []repositories.BuildpackRecord
		result2 error
	}{result1, result2}
}

func (fake *BuildpackRepository) GetBuildpacksForBuilderReturnsOnCall(i int, result1 []repositories.BuildpackRecord, result2 error) {
	fake.getBuildpacksForBuilderMutex.Lock()
	defer fake.getBuildpacksForBuilderMutex.Unlock()
	fake.GetBuildpacksForBuilderStub = nil
	if fake.getBuildpacksForBuilderReturnsOnCall == nil {
		fake.getBuildpacksForBuilderReturnsOnCall = make(map[int]struct {
			result1 []repositories.BuildpackRecord
			result2 error
		})
	}
	fake.getBuildpacksForBuilderReturnsOnCall[i] = struct {
		result1 []repositories.BuildpackRecord
		result2 error
	}{result1, result2}
}

func (fake *BuildpackRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.getBuildpacksForBuilderMutex.RLock()
	defer fake.getBuildpacksForBuilderMutex.RUnlock()
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
