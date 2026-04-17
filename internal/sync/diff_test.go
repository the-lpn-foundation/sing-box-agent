package sync

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInterfaceEqual tests the interfaceEqual helper function
func TestInterfaceEqual(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		{
			name: "equal - simple strings",
			a:    "hello",
			b:    "hello",
			want: true,
		},
		{
			name: "not equal - different strings",
			a:    "hello",
			b:    "world",
			want: false,
		},
		{
			name: "equal - integers",
			a:    42,
			b:    42,
			want: true,
		},
		{
			name: "not equal - different integers",
			a:    42,
			b:    43,
			want: false,
		},
		{
			name: "equal - booleans",
			a:    true,
			b:    true,
			want: true,
		},
		{
			name: "not equal - different booleans",
			a:    true,
			b:    false,
			want: false,
		},
		{
			name: "equal - floats",
			a:    3.14,
			b:    3.14,
			want: true,
		},
		{
			name: "not equal - different floats",
			a:    3.14,
			b:    3.15,
			want: false,
		},
		{
			name: "equal - nil values",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "not equal - one nil",
			a:    nil,
			b:    "value",
			want: false,
		},
		{
			name: "equal - empty maps",
			a:    map[string]interface{}{},
			b:    map[string]interface{}{},
			want: true,
		},
		{
			name: "equal - maps with same content",
			a:    map[string]interface{}{"key1": "value1", "key2": 42},
			b:    map[string]interface{}{"key1": "value1", "key2": 42},
			want: true,
		},
		{
			name: "not equal - maps with different keys",
			a:    map[string]interface{}{"key1": "value1"},
			b:    map[string]interface{}{"key2": "value2"},
			want: false,
		},
		{
			name: "not equal - maps with different values",
			a:    map[string]interface{}{"key1": "value1"},
			b:    map[string]interface{}{"key1": "value2"},
			want: false,
		},
		{
			name: "not equal - maps with different lengths",
			a:    map[string]interface{}{"key1": "value1", "key2": "value2"},
			b:    map[string]interface{}{"key1": "value1"},
			want: false,
		},
		{
			name: "not equal - map vs non-map",
			a:    map[string]interface{}{"key": "value"},
			b:    "not a map",
			want: false,
		},
		{
			name: "equal - nested maps",
			a:    map[string]interface{}{"outer": map[string]interface{}{"inner": "value"}},
			b:    map[string]interface{}{"outer": map[string]interface{}{"inner": "value"}},
			want: true,
		},
		{
			name: "not equal - nested maps with different values",
			a:    map[string]interface{}{"outer": map[string]interface{}{"inner": "value1"}},
			b:    map[string]interface{}{"outer": map[string]interface{}{"inner": "value2"}},
			want: false,
		},
		{
			name: "equal - empty slices",
			a:    []interface{}{},
			b:    []interface{}{},
			want: true,
		},
		{
			name: "equal - slices with same content",
			a:    []interface{}{"a", "b", 42},
			b:    []interface{}{"a", "b", 42},
			want: true,
		},
		{
			name: "not equal - slices with different lengths",
			a:    []interface{}{"a", "b"},
			b:    []interface{}{"a", "b", "c"},
			want: false,
		},
		{
			name: "not equal - slices with different values",
			a:    []interface{}{"a", "b"},
			b:    []interface{}{"a", "c"},
			want: false,
		},
		{
			name: "not equal - slice vs non-slice",
			a:    []interface{}{"a", "b"},
			b:    "not a slice",
			want: false,
		},
		{
			name: "equal - nested slices",
			a:    []interface{}{[]interface{}{"a", "b"}, []interface{}{"c", "d"}},
			b:    []interface{}{[]interface{}{"a", "b"}, []interface{}{"c", "d"}},
			want: true,
		},
		{
			name: "not equal - nested slices with different values",
			a:    []interface{}{[]interface{}{"a", "b"}},
			b:    []interface{}{[]interface{}{"a", "c"}},
			want: false,
		},
		{
			name: "equal - mixed nested structures",
			a:    map[string]interface{}{"key": []interface{}{map[string]interface{}{"inner": "value"}}},
			b:    map[string]interface{}{"key": []interface{}{map[string]interface{}{"inner": "value"}}},
			want: true,
		},
		{
			name: "not equal - mixed nested structures with different values",
			a:    map[string]interface{}{"key": []interface{}{map[string]interface{}{"inner": "value1"}}},
			b:    map[string]interface{}{"key": []interface{}{map[string]interface{}{"inner": "value2"}}},
			want: false,
		},
		{
			name: "equal - zero values",
			a:    0,
			b:    0,
			want: true,
		},
		{
			name: "equal - empty string",
			a:    "",
			b:    "",
			want: true,
		},
		{
			name: "not equal - different types",
			a:    42,
			b:    "42",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interfaceEqual(tt.a, tt.b)
			require.Equal(t, tt.want, got)
		})
	}
}
