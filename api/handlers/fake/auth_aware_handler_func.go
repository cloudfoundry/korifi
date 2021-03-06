// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"net/http"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"github.com/go-logr/logr"
)

type AuthAwareHandlerFunc struct {
	Stub        func(context.Context, logr.Logger, authorization.Info, *http.Request) (*handlers.HandlerResponse, error)
	mutex       sync.RWMutex
	argsForCall []struct {
		arg1 context.Context
		arg2 logr.Logger
		arg3 authorization.Info
		arg4 *http.Request
	}
	returns struct {
		result1 *handlers.HandlerResponse
		result2 error
	}
	returnsOnCall map[int]struct {
		result1 *handlers.HandlerResponse
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *AuthAwareHandlerFunc) Spy(arg1 context.Context, arg2 logr.Logger, arg3 authorization.Info, arg4 *http.Request) (*handlers.HandlerResponse, error) {
	fake.mutex.Lock()
	ret, specificReturn := fake.returnsOnCall[len(fake.argsForCall)]
	fake.argsForCall = append(fake.argsForCall, struct {
		arg1 context.Context
		arg2 logr.Logger
		arg3 authorization.Info
		arg4 *http.Request
	}{arg1, arg2, arg3, arg4})
	stub := fake.Stub
	returns := fake.returns
	fake.recordInvocation("AuthAwareHandlerFunc", []interface{}{arg1, arg2, arg3, arg4})
	fake.mutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3, arg4)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return returns.result1, returns.result2
}

func (fake *AuthAwareHandlerFunc) CallCount() int {
	fake.mutex.RLock()
	defer fake.mutex.RUnlock()
	return len(fake.argsForCall)
}

func (fake *AuthAwareHandlerFunc) Calls(stub func(context.Context, logr.Logger, authorization.Info, *http.Request) (*handlers.HandlerResponse, error)) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Stub = stub
}

func (fake *AuthAwareHandlerFunc) ArgsForCall(i int) (context.Context, logr.Logger, authorization.Info, *http.Request) {
	fake.mutex.RLock()
	defer fake.mutex.RUnlock()
	return fake.argsForCall[i].arg1, fake.argsForCall[i].arg2, fake.argsForCall[i].arg3, fake.argsForCall[i].arg4
}

func (fake *AuthAwareHandlerFunc) Returns(result1 *handlers.HandlerResponse, result2 error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Stub = nil
	fake.returns = struct {
		result1 *handlers.HandlerResponse
		result2 error
	}{result1, result2}
}

func (fake *AuthAwareHandlerFunc) ReturnsOnCall(i int, result1 *handlers.HandlerResponse, result2 error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Stub = nil
	if fake.returnsOnCall == nil {
		fake.returnsOnCall = make(map[int]struct {
			result1 *handlers.HandlerResponse
			result2 error
		})
	}
	fake.returnsOnCall[i] = struct {
		result1 *handlers.HandlerResponse
		result2 error
	}{result1, result2}
}

func (fake *AuthAwareHandlerFunc) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.mutex.RLock()
	defer fake.mutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *AuthAwareHandlerFunc) recordInvocation(key string, args []interface{}) {
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

var _ handlers.AuthAwareHandlerFunc = new(AuthAwareHandlerFunc).Spy
