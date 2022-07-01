//go:build tools
// +build tools

package tools

import (
	_ "github.com/maxbrunsfeld/counterfeiter/v6"
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)

// This file imports packages that are used when running go generate, or used
// during the development process but not otherwise depended on by built code.
