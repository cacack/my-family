package api

import (
	"github.com/oapi-codegen/nullable"

	"github.com/cacack/my-family/internal/command"
)

// patchOf turns an optional field of an update request into the command's
// set/clear/omit value (#1359): a field left out is omitted, an explicit null
// clears it, and a value sets it.
func patchOf[T any](n nullable.Nullable[T]) command.Patch[T] {
	return patchMap(n, func(v T) T { return v })
}

// patchString is patchOf for a field whose generated type is a named string
// (an enum or DateInput) and whose command field is a plain string.
func patchString[S ~string](n nullable.Nullable[S]) command.Patch[string] {
	return patchMap(n, func(v S) string { return string(v) })
}

// patchMap is patchOf with the set value converted by conv.
func patchMap[A, B any](n nullable.Nullable[A], conv func(A) B) command.Patch[B] {
	switch {
	case !n.IsSpecified():
		return command.Patch[B]{}
	case n.IsNull():
		return command.Clear[B]()
	default:
		return command.Set(conv(n.MustGet()))
	}
}

// omitIfNil returns a response field holding *p, or one left out of the JSON
// (under omitempty) when p is nil.
func omitIfNil[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nil
	}
	return nullable.NewNullableWithValue(*p)
}

// nullIfNil returns a response field holding *p, or an explicit null when p
// is nil. Use it for a required nullable field, which has no omitempty.
func nullIfNil[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*p)
}
