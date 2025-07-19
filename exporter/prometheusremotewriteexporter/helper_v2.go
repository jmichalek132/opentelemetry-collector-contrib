// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewriteexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"

import (
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

func batchTimeSeriesV2(tsMap map[string]*writev2.TimeSeries, symbolsTable writev2.SymbolsTable, maxBatchByteSize int, state *batchTimeSeriesState) ([]*writev2.Request, error) {
	// Convert to generic TimeSeries map
	genericTsMap := make(map[string]TimeSeries, len(tsMap))
	for key, ts := range tsMap {
		genericTsMap[key] = ts
	}

	// Calculate symbols table size once since it's shared across batches
	symbolsSize := 0
	for _, symbol := range symbolsTable.Symbols() {
		symbolsSize += len(symbol)
	}

	// Use unified batching for time series with symbols table size as extra overhead
	converter := NewV2TimeSeriesConverter(symbolsTable)
	requests, err := batchTimeSeriesGeneric(genericTsMap, maxBatchByteSize, state, converter, symbolsSize)
	if err != nil {
		return nil, err
	}

	// Convert generic requests back to v2 requests
	v2Requests := make([]*writev2.Request, 0, len(requests))
	for _, req := range requests {
		v2Req := req.(*prwV2Request)
		v2Requests = append(v2Requests, v2Req.Request)
	}

	return v2Requests, nil
}

// orderBySampleTimestampV2 sorts v2 time series by timestamp - exposed for tests
func orderBySampleTimestampV2(tsArray []writev2.TimeSeries) []writev2.TimeSeries {
	converter := &V2TimeSeriesConverter{symbolsTable: writev2.SymbolsTable{}}
	return converter.SortTimeSeries(tsArray).([]writev2.TimeSeries)
}
