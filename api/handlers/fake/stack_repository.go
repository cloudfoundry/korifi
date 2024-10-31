package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type StackRepository struct {
	ListStacksStub        func(context.Context, authorization.Info) ([]repositories.StackRecord, error)
	listStacksMutex       sync.RWMutex
	listStacksArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
	}
	listStacksReturns struct {
		result1 []repositories.StackRecord
		result2 error
	}
	listStacksReturnsOnCall map[int]struct {
		result1 []repositories.StackRecord
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *StackRepository) ListStacks(arg1 context.Context, arg2 authorization.Info) ([]repositories.StackRecord, error) {
	fake.listStacksMutex.Lock()
	ret, specificReturn := fake.listStacksReturnsOnCall[len(fake.listStacksArgsForCall)]
	fake.listStacksArgsForCall = append(fake.listStacksArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
	}{arg1, arg2})
	stub := fake.ListStacksStub
	fakeReturns := fake.listStacksReturns
	fake.recordInvocation("ListStacks", []interface{}{arg1, arg2})
	fake.listStacksMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *StackRepository) ListStacksCallCount() int {
	fake.listStacksMutex.RLock()
	defer fake.listStacksMutex.RUnlock()
	return len(fake.listStacksArgsForCall)
}

func (fake *StackRepository) ListStacksCalls(stub func(context.Context, authorization.Info) ([]repositories.StackRecord, error)) {
	fake.listStacksMutex.Lock()
	defer fake.listStacksMutex.Unlock()
	fake.ListStacksStub = stub
}

func (fake *StackRepository) ListStacksArgsForCall(i int) (context.Context, authorization.Info) {
	fake.listStacksMutex.RLock()
	defer fake.listStacksMutex.RUnlock()
	argsForCall := fake.listStacksArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *StackRepository) ListStacksReturns(result1 []repositories.StackRecord, result2 error) {
	fake.listStacksMutex.Lock()
	defer fake.listStacksMutex.Unlock()
	fake.ListStacksStub = nil
	fake.listStacksReturns = struct {
		result1 []repositories.StackRecord
		result2 error
	}{result1, result2}
}

func (fake *StackRepository) ListStacksReturnsOnCall(i int, result1 []repositories.StackRecord, result2 error) {
	fake.listStacksMutex.Lock()
	defer fake.listStacksMutex.Unlock()
	fake.ListStacksStub = nil
	if fake.listStacksReturnsOnCall == nil {
		fake.listStacksReturnsOnCall = make(map[int]struct {
			result1 []repositories.StackRecord
			result2 error
		})
	}
	fake.listStacksReturnsOnCall[i] = struct {
		result1 []repositories.StackRecord
		result2 error
	}{result1, result2}
}

func (fake *StackRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.listStacksMutex.RLock()
	defer fake.listStacksMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *StackRepository) recordInvocation(key string, args []interface{}) {
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

var _ handlers.StackRepository = new(StackRepository)
