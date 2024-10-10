// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewrite

import (
	"bytes"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"net/http"
	"testing"
	"time"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestFromMetricsV2(t *testing.T) {
	settings := Settings{
		Namespace:           "",
		ExternalLabels:      nil,
		DisableTargetInfo:   false,
		ExportCreatedMetric: false,
		AddMetricSuffixes:   false,
		SendMetadata:        false,
	}

	ts := uint64(time.Now().UnixNano())
	payload := createExportRequest(5, 0, 1, 3, 0, pcommon.Timestamp(ts))
	want := func() map[string]*writev2.TimeSeries {
		return map[string]*writev2.TimeSeries{
			"0": {
				LabelsRefs: []uint32{1, 2},
				Samples: []writev2.Sample{
					{Timestamp: convertTimeStamp(pcommon.Timestamp(ts)), Value: 1.23},
				},
			},
		}
	}
	tsMap, symbolsTable, err := FromMetricsV2(payload.Metrics(), settings)
	wanted := want()
	require.NoError(t, err)
	require.NotNil(t, tsMap)
	require.Equal(t, wanted, tsMap)
	require.NotNil(t, symbolsTable)

}

func TestSendingFromMetricsV2(t *testing.T) {
	settings := Settings{
		Namespace:           "",
		ExternalLabels:      nil,
		DisableTargetInfo:   false,
		ExportCreatedMetric: false,
		AddMetricSuffixes:   false,
		SendMetadata:        false,
	}

	ts := uint64(time.Now().UnixNano())
	payload := createExportRequest(5, 0, 1, 3, 0, pcommon.Timestamp(ts))

	tsMap, symbolsTable, err := FromMetricsV2(payload.Metrics(), settings)
	require.NoError(t, err)

	tsArray := make([]writev2.TimeSeries, 0, len(tsMap))
	for _, ts := range tsMap {
		tsArray = append(tsArray, *ts)
	}

	writeReq := &writev2.Request{
		Symbols:    symbolsTable.Symbols(),
		Timeseries: tsArray,
	}

	data, errMarshal := proto.Marshal(writeReq)
	require.NoError(t, errMarshal)

	compressedData := snappy.Encode(nil, data)

	req, err := http.NewRequest("POST", "http://localhost:9091/api/v1/write", bytes.NewReader(compressedData))
	require.NoError(t, errMarshal)

	c := http.Client{}
	resp, err := c.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

}
