// +build tools

package tools

import (
_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)

// This file imports packages that are used when running go generate, or used
// during the development process but not otherwise depended on by built code.
