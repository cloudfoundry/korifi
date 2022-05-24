// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"io"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
)

type ImageRepository struct {
	UploadSourceImageStub        func(context.Context, authorization.Info, string, io.Reader, string) (string, error)
	uploadSourceImageMutex       sync.RWMutex
	uploadSourceImageArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
		arg4 io.Reader
		arg5 string
	}
	uploadSourceImageReturns struct {
		result1 string
		result2 error
	}
	uploadSourceImageReturnsOnCall map[int]struct {
		result1 string
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *ImageRepository) UploadSourceImage(arg1 context.Context, arg2 authorization.Info, arg3 string, arg4 io.Reader, arg5 string) (string, error) {
	fake.uploadSourceImageMutex.Lock()
	ret, specificReturn := fake.uploadSourceImageReturnsOnCall[len(fake.uploadSourceImageArgsForCall)]
	fake.uploadSourceImageArgsForCall = append(fake.uploadSourceImageArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
		arg4 io.Reader
		arg5 string
	}{arg1, arg2, arg3, arg4, arg5})
	stub := fake.UploadSourceImageStub
	fakeReturns := fake.uploadSourceImageReturns
	fake.recordInvocation("UploadSourceImage", []interface{}{arg1, arg2, arg3, arg4, arg5})
	fake.uploadSourceImageMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3, arg4, arg5)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *ImageRepository) UploadSourceImageCallCount() int {
	fake.uploadSourceImageMutex.RLock()
	defer fake.uploadSourceImageMutex.RUnlock()
	return len(fake.uploadSourceImageArgsForCall)
}

func (fake *ImageRepository) UploadSourceImageCalls(stub func(context.Context, authorization.Info, string, io.Reader, string) (string, error)) {
	fake.uploadSourceImageMutex.Lock()
	defer fake.uploadSourceImageMutex.Unlock()
	fake.UploadSourceImageStub = stub
}

func (fake *ImageRepository) UploadSourceImageArgsForCall(i int) (context.Context, authorization.Info, string, io.Reader, string) {
	fake.uploadSourceImageMutex.RLock()
	defer fake.uploadSourceImageMutex.RUnlock()
	argsForCall := fake.uploadSourceImageArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3, argsForCall.arg4, argsForCall.arg5
}

func (fake *ImageRepository) UploadSourceImageReturns(result1 string, result2 error) {
	fake.uploadSourceImageMutex.Lock()
	defer fake.uploadSourceImageMutex.Unlock()
	fake.UploadSourceImageStub = nil
	fake.uploadSourceImageReturns = struct {
		result1 string
		result2 error
	}{result1, result2}
}

func (fake *ImageRepository) UploadSourceImageReturnsOnCall(i int, result1 string, result2 error) {
	fake.uploadSourceImageMutex.Lock()
	defer fake.uploadSourceImageMutex.Unlock()
	fake.UploadSourceImageStub = nil
	if fake.uploadSourceImageReturnsOnCall == nil {
		fake.uploadSourceImageReturnsOnCall = make(map[int]struct {
			result1 string
			result2 error
		})
	}
	fake.uploadSourceImageReturnsOnCall[i] = struct {
		result1 string
		result2 error
	}{result1, result2}
}

func (fake *ImageRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.uploadSourceImageMutex.RLock()
	defer fake.uploadSourceImageMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *ImageRepository) recordInvocation(key string, args []interface{}) {
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

var _ handlers.ImageRepository = new(ImageRepository)
