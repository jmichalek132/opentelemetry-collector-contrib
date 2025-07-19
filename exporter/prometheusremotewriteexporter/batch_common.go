// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewriteexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"

import (
	"math"
)

// batchTimeSeriesState tracks buffer sizes to optimize memory allocation across batches.
// This is shared between v1 and v2 implementations to avoid duplication.
type batchTimeSeriesState struct {
	// Track batch sizes sent to avoid over allocating huge buffers.
	// This helps in the case where large batches are sent to avoid allocating too much unused memory
	nextTimeSeriesBufferSize     int
	nextMetricMetadataBufferSize int
	nextRequestBufferSize        int
}

func newBatchTimeServicesState() *batchTimeSeriesState {
	return &batchTimeSeriesState{
		nextTimeSeriesBufferSize:     math.MaxInt,
		nextMetricMetadataBufferSize: math.MaxInt,
		nextRequestBufferSize:        0,
	}
}

// calculateOptimalBufferSize returns the optimal buffer size for the next batch
// based on the previous batch size and the total number of items to process.
func calculateOptimalBufferSize(previousSize, totalItems int) int {
	return min(max(10, 2*previousSize), totalItems)
}
