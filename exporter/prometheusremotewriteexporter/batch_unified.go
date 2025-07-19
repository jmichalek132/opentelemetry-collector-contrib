// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewriteexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"

import (
	"errors"
	"sort"

	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// TimeSeries represents a generic time series that can be processed by both v1 and v2
type TimeSeries interface {
	Size() int
}

// TimeSeriesConverter converts time series arrays to requests
type TimeSeriesConverter interface {
	ConvertToRequest(tsArray interface{}) Request
	CreateArray(capacity int) interface{}
	AppendToArray(array interface{}, ts TimeSeries) interface{}
	SortTimeSeries(array interface{}) interface{}
}

// V1TimeSeriesConverter handles v1 time series conversion
type V1TimeSeriesConverter struct{}

func (c *V1TimeSeriesConverter) ConvertToRequest(tsArray interface{}) Request {
	array := tsArray.([]prompb.TimeSeries)
	return &prwV1Request{
		WriteRequest: &prompb.WriteRequest{
			Timeseries: c.SortTimeSeries(array).([]prompb.TimeSeries),
		},
	}
}

func (c *V1TimeSeriesConverter) CreateArray(capacity int) interface{} {
	return make([]prompb.TimeSeries, 0, capacity)
}

func (c *V1TimeSeriesConverter) AppendToArray(array interface{}, ts TimeSeries) interface{} {
	arr := array.([]prompb.TimeSeries)
	return append(arr, *(ts.(*prompb.TimeSeries)))
}

func (c *V1TimeSeriesConverter) SortTimeSeries(array interface{}) interface{} {
	tsArray := array.([]prompb.TimeSeries)
	for i := range tsArray {
		sL := tsArray[i].Samples
		sort.Slice(sL, func(i, j int) bool {
			return sL[i].Timestamp < sL[j].Timestamp
		})
	}
	return tsArray
}

// V2TimeSeriesConverter handles v2 time series conversion
type V2TimeSeriesConverter struct {
	symbolsTable writev2.SymbolsTable
}

func NewV2TimeSeriesConverter(symbolsTable writev2.SymbolsTable) *V2TimeSeriesConverter {
	return &V2TimeSeriesConverter{symbolsTable: symbolsTable}
}

func (c *V2TimeSeriesConverter) ConvertToRequest(tsArray interface{}) Request {
	array := tsArray.([]writev2.TimeSeries)
	return &prwV2Request{
		Request: &writev2.Request{
			Timeseries: c.SortTimeSeries(array).([]writev2.TimeSeries),
			Symbols:    c.symbolsTable.Symbols(),
		},
	}
}

func (c *V2TimeSeriesConverter) CreateArray(capacity int) interface{} {
	return make([]writev2.TimeSeries, 0, capacity)
}

func (c *V2TimeSeriesConverter) AppendToArray(array interface{}, ts TimeSeries) interface{} {
	arr := array.([]writev2.TimeSeries)
	return append(arr, *(ts.(*writev2.TimeSeries)))
}

func (c *V2TimeSeriesConverter) SortTimeSeries(array interface{}) interface{} {
	tsArray := array.([]writev2.TimeSeries)
	for i := range tsArray {
		sL := tsArray[i].Samples
		sort.Slice(sL, func(i, j int) bool {
			return sL[i].Timestamp < sL[j].Timestamp
		})
	}
	return tsArray
}

// batchTimeSeriesGeneric is a unified batching function that works for both v1 and v2
func batchTimeSeriesGeneric(tsMap map[string]TimeSeries, maxBatchByteSize int, state *batchTimeSeriesState, converter TimeSeriesConverter, extraSize int) ([]Request, error) {
	if len(tsMap) == 0 {
		return nil, errors.New("invalid tsMap: cannot be empty map")
	}

	requests := make([]Request, 0, max(10, state.nextRequestBufferSize))
	tsArray := converter.CreateArray(calculateOptimalBufferSize(state.nextTimeSeriesBufferSize, len(tsMap)))

	sizeOfCurrentBatch := extraSize // Initialize with extra size (e.g., symbols table size for v2)
	i := 0

	for _, v := range tsMap {
		sizeOfSeries := v.Size()

		if sizeOfCurrentBatch+sizeOfSeries >= maxBatchByteSize {
			state.nextTimeSeriesBufferSize = max(10, 2*getArrayLength(tsArray))
			wrapped := converter.ConvertToRequest(tsArray)
			requests = append(requests, wrapped)

			tsArray = converter.CreateArray(calculateOptimalBufferSize(state.nextTimeSeriesBufferSize, len(tsMap)-i))
			sizeOfCurrentBatch = extraSize // Reset to extra size for new batch
		}

		tsArray = converter.AppendToArray(tsArray, v)
		sizeOfCurrentBatch += sizeOfSeries
		i++
	}

	if getArrayLength(tsArray) != 0 {
		wrapped := converter.ConvertToRequest(tsArray)
		requests = append(requests, wrapped)
	}

	state.nextRequestBufferSize = 2 * len(requests)
	return requests, nil
}

// Helper function to get array length regardless of type
func getArrayLength(array interface{}) int {
	switch a := array.(type) {
	case []prompb.TimeSeries:
		return len(a)
	case []writev2.TimeSeries:
		return len(a)
	default:
		return 0
	}
}
