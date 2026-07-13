// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/juju/juju/api/client/action"
)

// TestActionResultToOutputMap verifies that a Juju action result's output is
// converted into a dynamic value that preserves the nested structure.
func TestActionResultToOutputMap(t *testing.T) {
	ctx := context.Background()

	// A nil or empty output produces a null dynamic value.
	got, err := actionResultToOutputMap(ctx, action.ActionResult{})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !got.IsNull() {
		t.Fatalf("expected null dynamic value, got %s", got.String())
	}

	// A nested output is preserved as a dynamic object with nested maps,
	// lists and scalars.
	result := action.ActionResult{
		Output: map[string]any{
			"endpoint": "https://example.com",
			"port":     float64(8080),
			"enabled":  true,
			"nested": map[string]any{
				"key": "value",
			},
			"list": []any{"a", "b"},
		},
	}
	got, err = actionResultToOutputMap(ctx, result)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got.IsNull() || got.IsUnknown() {
		t.Fatalf("expected known dynamic value, got %s", got.String())
	}

	obj, ok := got.UnderlyingValue().(types.Object)
	if !ok {
		t.Fatalf("expected underlying object value, got %T", got.UnderlyingValue())
	}
	attrs := obj.Attributes()

	endpoint, ok := attrs["endpoint"].(types.String)
	if !ok {
		t.Fatalf("expected endpoint to be a string, got %T", attrs["endpoint"])
	}
	if endpoint.ValueString() != "https://example.com" {
		t.Fatalf("unexpected endpoint: %s", endpoint.ValueString())
	}

	enabled, ok := attrs["enabled"].(types.Bool)
	if !ok {
		t.Fatalf("expected enabled to be a bool, got %T", attrs["enabled"])
	}
	if !enabled.ValueBool() {
		t.Fatalf("expected enabled to be true")
	}

	nested, ok := attrs["nested"].(types.Object)
	if !ok {
		t.Fatalf("expected nested to be an object, got %T", attrs["nested"])
	}
	nestedKey, ok := nested.Attributes()["key"].(types.String)
	if !ok {
		t.Fatalf("expected nested.key to be a string, got %T", nested.Attributes()["key"])
	}
	if nestedKey.ValueString() != "value" {
		t.Fatalf("unexpected nested.key: %s", nestedKey.ValueString())
	}

	list, ok := attrs["list"].(types.Tuple)
	if !ok {
		t.Fatalf("expected list to be a tuple, got %T", attrs["list"])
	}
	elems := list.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 list elements, got %d", len(elems))
	}
	if _, ok := elems[0].(types.String); !ok {
		t.Fatalf("expected list element to be a string, got %T", elems[0])
	}

	// Ensure the port scalar round-trips as a number.
	if _, ok := attrs["port"].(attr.Value); !ok {
		t.Fatalf("expected port to be an attr.Value")
	}
}
