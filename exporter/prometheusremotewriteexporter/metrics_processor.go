// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewriteexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"

import (
	"context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/prompb"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheusremotewrite"
)

// MetricsProcessor defines the interface for processing metrics in different formats
type MetricsProcessor interface {
	ProcessMetrics(ctx context.Context, md pmetric.Metrics) error
}

// V1MetricsProcessor handles Prometheus Remote Write v1 format
type V1MetricsProcessor struct {
	exporter *prwExporter
}

func NewV1MetricsProcessor(exporter *prwExporter) *V1MetricsProcessor {
	return &V1MetricsProcessor{exporter: exporter}
}

func (p *V1MetricsProcessor) ProcessMetrics(ctx context.Context, md pmetric.Metrics) error {
	tsMap, err := prometheusremotewrite.FromMetrics(md, p.exporter.exporterSettings)

	p.exporter.telemetry.recordTranslatedTimeSeries(ctx, len(tsMap))

	var m []*prompb.MetricMetadata
	if p.exporter.exporterSettings.SendMetadata {
		m = prometheusremotewrite.OtelMetricsToMetadata(md, p.exporter.exporterSettings.AddMetricSuffixes, p.exporter.exporterSettings.Namespace)
	}
	if err != nil {
		p.exporter.telemetry.recordTranslationFailure(ctx)
		p.exporter.settings.Logger.Debug("failed to translate metrics, exporting remaining metrics", zap.Error(err), zap.Int("translated", len(tsMap)))
	}
	// Call export even if a conversion error, since there may be points that were successfully converted.
	return p.exporter.handleExport(ctx, tsMap, m)
}

// V2MetricsProcessor handles Prometheus Remote Write v2 format
type V2MetricsProcessor struct {
	exporter *prwExporter
}

func NewV2MetricsProcessor(exporter *prwExporter) *V2MetricsProcessor {
	return &V2MetricsProcessor{exporter: exporter}
}

func (p *V2MetricsProcessor) ProcessMetrics(ctx context.Context, md pmetric.Metrics) error {
	tsMap, symbolsTable, err := prometheusremotewrite.FromMetricsV2(md, p.exporter.exporterSettings)

	p.exporter.telemetry.recordTranslatedTimeSeries(ctx, len(tsMap))

	if err != nil {
		p.exporter.telemetry.recordTranslationFailure(ctx)
		p.exporter.settings.Logger.Debug("failed to translate metrics, exporting remaining metrics", zap.Error(err), zap.Int("translated", len(tsMap)))
	}
	// Call export even if a conversion error, since there may be points that were successfully converted.
	return p.exporter.handleExportV2(ctx, symbolsTable, tsMap)
}

// getMetricsProcessor returns the appropriate metrics processor based on configuration
func (prwe *prwExporter) getMetricsProcessor() MetricsProcessor {
	// If feature flag not enabled support only RW1.
	if !enableSendingRW2FeatureGate.IsEnabled() {
		return NewV1MetricsProcessor(prwe)
	}

	// If feature flag was enabled check if we want to send RW1 or RW2.
	switch prwe.RemoteWriteProtoMsg {
	case config.RemoteWriteProtoMsgV1:
		return NewV1MetricsProcessor(prwe)
	case config.RemoteWriteProtoMsgV2:
		return NewV2MetricsProcessor(prwe)
	default:
		// Default to v1 if unknown
		return NewV1MetricsProcessor(prwe)
	}
}
