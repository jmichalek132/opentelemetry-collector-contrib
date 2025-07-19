// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewriteexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"

import (
	"context"
	"net/http"
	"strconv"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"go.uber.org/zap"
)

// prwV2Request wraps writev2.Request to implement the Request interface
type prwV2Request struct {
	*writev2.Request
}

func (r *prwV2Request) Marshal() ([]byte, error) {
	return r.Request.Marshal()
}

func (r *prwV2Request) Size() int {
	return r.Request.Size()
}

// exportV2 sends a Snappy-compressed writev2.Request containing writev2.TimeSeries to a remote write endpoint.
func (prwe *prwExporter) exportV2(ctx context.Context, requests []*writev2.Request) error {
	// Convert v2 requests to the generic Request interface
	genericRequests := make([]Request, len(requests))
	for i, req := range requests {
		genericRequests[i] = &prwV2Request{Request: req}
	}

	// Use the common export logic
	return prwe.exportRequests(ctx, genericRequests)
}

func (prwe *prwExporter) handleExportV2(ctx context.Context, symbolsTable writev2.SymbolsTable, tsMap map[string]*writev2.TimeSeries) error {
	// There are no metrics to export, so return.
	if len(tsMap) == 0 {
		return nil
	}

	state := prwe.batchStatePool.Get().(*batchTimeSeriesState)
	defer prwe.batchStatePool.Put(state)
	requests, err := batchTimeSeriesV2(tsMap, symbolsTable, prwe.maxBatchSizeBytes, state)
	if err != nil {
		return err
	}

	// TODO implement WAL support, can be done after #15277 is fixed

	return prwe.exportV2(ctx, requests)
}

func (prwe *prwExporter) handleHeader(ctx context.Context, resp *http.Response, headerName, metricType string, recordFunc func(context.Context, int64)) {
	headerValue := resp.Header.Get(headerName)
	if headerValue == "" {
		prwe.settings.Logger.Warn(
			headerName+" header is missing from the response, suggesting that the endpoint doesn't support RW2 and might be silently dropping data.",
			zap.String("url", resp.Request.URL.String()),
		)
		return
	}

	value, err := strconv.ParseInt(headerValue, 10, 64)
	if err != nil {
		prwe.settings.Logger.Warn(
			"Failed to convert "+headerName+" header to int64, not counting "+metricType+" written",
			zap.String("url", resp.Request.URL.String()),
		)
		return
	}
	recordFunc(ctx, value)
}

func (prwe *prwExporter) handleWrittenHeaders(ctx context.Context, resp *http.Response) {
	prwe.handleHeader(ctx, resp,
		"X-Prometheus-Remote-Write-Samples-Written",
		"samples",
		prwe.telemetry.recordWrittenSamples)

	prwe.handleHeader(ctx, resp,
		"X-Prometheus-Remote-Write-Histograms-Written",
		"histograms",
		prwe.telemetry.recordWrittenHistograms)

	prwe.handleHeader(ctx, resp,
		"X-Prometheus-Remote-Write-Exemplars-Written",
		"exemplars",
		prwe.telemetry.recordWrittenExemplars)
}
