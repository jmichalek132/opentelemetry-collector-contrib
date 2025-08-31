package metrics

import (
	"testing"
)

func TestMetricTypeSet_NewTypes(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:    "ExponentialHistogram",
			value:   "ExponentialHistogram",
			wantErr: false,
		},
		{
			name:    "Summary",
			value:   "Summary",
			wantErr: false,
		},
		{
			name:    "Invalid type",
			value:   "InvalidType",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mt MetricType
			err := mt.Set(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("MetricType.Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(mt) != tt.value {
				t.Errorf("MetricType.Set() = %v, want %v", string(mt), tt.value)
			}
		})
	}
}
