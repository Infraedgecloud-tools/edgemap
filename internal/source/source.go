// Package source defines the adapter boundary for infrastructure inputs.
package source

import "context"

// Attribute retains static values and references without evaluating configuration.
type Attribute struct {
	Value      any
	Literal    bool
	Raw        string
	References []string
}

// Block is an infrastructure declaration. Attributes include nested block paths.
type Block struct {
	Kind       string
	Labels     []string
	Attributes map[string]Attribute
	File       string
}

// Result contains declarations and recoverable diagnostics.
type Result struct {
	Blocks   []Block
	Warnings []string
}

// Source reads one configuration directory without credentials or execution.
type Source interface {
	Load(context.Context, string) (Result, error)
}
