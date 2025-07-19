// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewriteexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"

import (
	"github.com/prometheus/prometheus/prompb"
)

// batchTimeSeries splits series into multiple batch write requests using the unified batching logic.
func batchTimeSeries(tsMap map[string]*prompb.TimeSeries, maxBatchByteSize int, m []*prompb.MetricMetadata, state *batchTimeSeriesState) ([]*prompb.WriteRequest, error) {
	// Convert to generic TimeSeries map
	genericTsMap := make(map[string]TimeSeries, len(tsMap))
	for key, ts := range tsMap {
		genericTsMap[key] = ts
	}

	// Use unified batching for time series
	converter := &V1TimeSeriesConverter{}
	requests, err := batchTimeSeriesGeneric(genericTsMap, maxBatchByteSize, state, converter, 0)
	if err != nil {
		return nil, err
	}

	// Convert generic requests back to v1 requests
	v1Requests := make([]*prompb.WriteRequest, 0, len(requests))
	for _, req := range requests {
		v1Req := req.(*prwV1Request)
		v1Requests = append(v1Requests, v1Req.WriteRequest)
	}

	// Handle metadata batching if needed
	if len(m) > 0 {
		metadataRequests, err := batchMetadata(m, maxBatchByteSize, state)
		if err != nil {
			return nil, err
		}
		v1Requests = append(v1Requests, metadataRequests...)
	}

	return v1Requests, nil
}

// batchMetadata handles metadata batching separately since it's v1-specific
func batchMetadata(m []*prompb.MetricMetadata, maxBatchByteSize int, state *batchTimeSeriesState) ([]*prompb.WriteRequest, error) {
	if len(m) == 0 {
		return nil, nil
	}

	var requests []*prompb.WriteRequest
	mArray := make([]prompb.MetricMetadata, 0, calculateOptimalBufferSize(state.nextMetricMetadataBufferSize, len(m)))
	sizeOfCurrentBatch := 0

	for i, v := range m {
		sizeOfM := v.Size()

		if sizeOfCurrentBatch+sizeOfM >= maxBatchByteSize {
			state.nextMetricMetadataBufferSize = max(10, 2*len(mArray))
			wrapped := &prompb.WriteRequest{Metadata: mArray}
			requests = append(requests, wrapped)

			mArray = make([]prompb.MetricMetadata, 0, calculateOptimalBufferSize(state.nextMetricMetadataBufferSize, len(m)-i))
			sizeOfCurrentBatch = 0
		}

		mArray = append(mArray, *v)
		sizeOfCurrentBatch += sizeOfM
	}

	if len(mArray) != 0 {
		wrapped := &prompb.WriteRequest{Metadata: mArray}
		requests = append(requests, wrapped)
	}

	return requests, nil
}

// orderBySampleTimestamp sorts time series by timestamp - exposed for tests
func orderBySampleTimestamp(tsArray []prompb.TimeSeries) []prompb.TimeSeries {
	converter := &V1TimeSeriesConverter{}
	return converter.SortTimeSeries(tsArray).([]prompb.TimeSeries)
}

// convertTimeseriesToRequest converts time series array to WriteRequest - exposed for tests
func convertTimeseriesToRequest(tsArray []prompb.TimeSeries) *prompb.WriteRequest {
	return &prompb.WriteRequest{
		Timeseries: orderBySampleTimestamp(tsArray),
	}
}
