// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type UserRepository struct {
	ListUsersStub        func(context.Context, authorization.Info, repositories.ListUsersMessage) (repositories.ListResult[repositories.UserRecord], error)
	listUsersMutex       sync.RWMutex
	listUsersArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListUsersMessage
	}
	listUsersReturns struct {
		result1 repositories.ListResult[repositories.UserRecord]
		result2 error
	}
	listUsersReturnsOnCall map[int]struct {
		result1 repositories.ListResult[repositories.UserRecord]
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *UserRepository) ListUsers(arg1 context.Context, arg2 authorization.Info, arg3 repositories.ListUsersMessage) (repositories.ListResult[repositories.UserRecord], error) {
	fake.listUsersMutex.Lock()
	ret, specificReturn := fake.listUsersReturnsOnCall[len(fake.listUsersArgsForCall)]
	fake.listUsersArgsForCall = append(fake.listUsersArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 repositories.ListUsersMessage
	}{arg1, arg2, arg3})
	stub := fake.ListUsersStub
	fakeReturns := fake.listUsersReturns
	fake.recordInvocation("ListUsers", []interface{}{arg1, arg2, arg3})
	fake.listUsersMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *UserRepository) ListUsersCallCount() int {
	fake.listUsersMutex.RLock()
	defer fake.listUsersMutex.RUnlock()
	return len(fake.listUsersArgsForCall)
}

func (fake *UserRepository) ListUsersCalls(stub func(context.Context, authorization.Info, repositories.ListUsersMessage) (repositories.ListResult[repositories.UserRecord], error)) {
	fake.listUsersMutex.Lock()
	defer fake.listUsersMutex.Unlock()
	fake.ListUsersStub = stub
}

func (fake *UserRepository) ListUsersArgsForCall(i int) (context.Context, authorization.Info, repositories.ListUsersMessage) {
	fake.listUsersMutex.RLock()
	defer fake.listUsersMutex.RUnlock()
	argsForCall := fake.listUsersArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *UserRepository) ListUsersReturns(result1 repositories.ListResult[repositories.UserRecord], result2 error) {
	fake.listUsersMutex.Lock()
	defer fake.listUsersMutex.Unlock()
	fake.ListUsersStub = nil
	fake.listUsersReturns = struct {
		result1 repositories.ListResult[repositories.UserRecord]
		result2 error
	}{result1, result2}
}

func (fake *UserRepository) ListUsersReturnsOnCall(i int, result1 repositories.ListResult[repositories.UserRecord], result2 error) {
	fake.listUsersMutex.Lock()
	defer fake.listUsersMutex.Unlock()
	fake.ListUsersStub = nil
	if fake.listUsersReturnsOnCall == nil {
		fake.listUsersReturnsOnCall = make(map[int]struct {
			result1 repositories.ListResult[repositories.UserRecord]
			result2 error
		})
	}
	fake.listUsersReturnsOnCall[i] = struct {
		result1 repositories.ListResult[repositories.UserRecord]
		result2 error
	}{result1, result2}
}

func (fake *UserRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *UserRepository) recordInvocation(key string, args []interface{}) {
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

var _ handlers.UserRepository = new(UserRepository)
