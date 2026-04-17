package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateInboundCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "valid request",
			data:    `{"tag":"trojan-in","type":"trojan","listen":"0.0.0.0:443","options":{}}`,
			wantErr: false,
		},
		{
			name:    "missing tag",
			data:    `{"type":"trojan","listen":"0.0.0.0:443"}`,
			wantErr: true,
		},
		{
			name:    "missing type",
			data:    `{"tag":"trojan-in","listen":"0.0.0.0:443"}`,
			wantErr: true,
		},
		{
			name:    "missing listen",
			data:    `{"tag":"trojan-in","type":"trojan"}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			data:    `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInboundCreate([]byte(tt.data))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateUserCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "valid request",
			data:    `{"subId":"user-123","email":"user@example.com"}`,
			wantErr: false,
		},
		{
			name:    "missing subId",
			data:    `{"email":"user@example.com"}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			data:    `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUserCreate([]byte(tt.data))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
