package labels

// Compiler is a reusable map composer. It persists a set of defaults, which
// can be overridden when calling Compile() to produce the combined map.
type Compiler struct {
	defaults map[string]string
}

func NewCompiler() Compiler {
	return Compiler{
		defaults: map[string]string{},
	}
}

func (o Compiler) Defaults(defaults map[string]string) Compiler {
	defaultsCopy := copyMap(o.defaults)
	for k, v := range defaults {
		defaultsCopy[k] = v
	}
	return Compiler{
		defaults: defaultsCopy,
	}
}

func (o Compiler) Compile(overrides map[string]string) map[string]string {
	res := map[string]string{}
	for k, v := range o.defaults {
		res[k] = v
	}
	for k, v := range overrides {
		res[k] = v
	}

	return res
}

func copyMap(src map[string]string) map[string]string {
	dst := map[string]string{}
	for k, v := range src {
		dst[k] = v
	}

	return dst
}
