// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewrite // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheusremotewrite"

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	conventions "go.opentelemetry.io/otel/semconv/v1.25.0"
)

// addResourceTargetInfoV2 converts the resource to the target info metric.
func (c *prometheusConverterV2) addResourceTargetInfoV2(resource pcommon.Resource, settings Settings, timestamp pcommon.Timestamp) {
	if settings.DisableTargetInfo || timestamp == 0 {
		return
	}

	attributes := resource.Attributes()
	identifyingAttrs := []string{
		string(conventions.ServiceNamespaceKey),
		string(conventions.ServiceNameKey),
		string(conventions.ServiceInstanceIDKey),
	}
	nonIdentifyingAttrsCount := attributes.Len()
	for _, a := range identifyingAttrs {
		_, haveAttr := attributes.Get(a)
		if haveAttr {
			nonIdentifyingAttrsCount--
		}
	}
	if nonIdentifyingAttrsCount == 0 {
		// If we only have job + instance, then target_info isn't useful, so don't add it.
		return
	}

	name := otlptranslator.TargetInfoMetricName
	if settings.Namespace != "" {
		// TODO what to do with this in case of full utf-8 support?
		name = settings.Namespace + "_" + name
	}

	labels := createAttributes(resource, attributes, settings.ExternalLabels, identifyingAttrs, false, c.labelNamer, model.MetricNameLabel, name)
	haveIdentifier := false
	for _, l := range labels {
		if l.Name == model.JobLabel || l.Name == model.InstanceLabel {
			haveIdentifier = true
			break
		}
	}

	if !haveIdentifier {
		// We need at least one identifying label to generate target_info.
		return
	}

	sample := &writev2.Sample{
		Value: float64(1),
		// convert ns to ms
		Timestamp: convertTimeStamp(timestamp),
	}
	c.addSample(sample, labels, metadata{
		Type: writev2.Metadata_METRIC_TYPE_GAUGE,
		Help: "Target metadata",
	})
}

// addSampleWithLabels is a helper function to create and add a sample with labels
func (c *prometheusConverterV2) addSampleWithLabels(sampleValue float64, timestamp int64, noRecordedValue bool,
	baseName string, baseLabels []prompb.Label, labelName, labelValue string, metadata metadata,
) {
	sample := &writev2.Sample{
		Value:     sampleValue,
		Timestamp: timestamp,
	}
	if noRecordedValue {
		sample.Value = math.Float64frombits(value.StaleNaN)
	}
	if labelName != "" && labelValue != "" {
		c.addSample(sample, createLabels(baseName, baseLabels, labelName, labelValue), metadata)
	} else {
		c.addSample(sample, createLabels(baseName, baseLabels), metadata)
	}
}

func (c *prometheusConverterV2) addSummaryDataPoints(dataPoints pmetric.SummaryDataPointSlice, resource pcommon.Resource,
	settings Settings, baseName string, metadata metadata,
) {
	for x := 0; x < dataPoints.Len(); x++ {
		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		baseLabels := createAttributes(resource, pt.Attributes(), settings.ExternalLabels, nil, false, c.labelNamer)
		noRecordedValue := pt.Flags().NoRecordedValue()

		// Add sum and count samples
		c.addSampleWithLabels(pt.Sum(), timestamp, noRecordedValue, baseName+sumStr, baseLabels, "", "", metadata)
		c.addSampleWithLabels(float64(pt.Count()), timestamp, noRecordedValue, baseName+countStr, baseLabels, "", "", metadata)

		// Process quantiles
		for i := 0; i < pt.QuantileValues().Len(); i++ {
			qt := pt.QuantileValues().At(i)
			percentileStr := strconv.FormatFloat(qt.Quantile(), 'f', -1, 64)
			c.addSampleWithLabels(qt.Value(), timestamp, noRecordedValue, baseName, baseLabels, quantileStr, percentileStr, metadata)
		}
	}
}

func (c *prometheusConverterV2) addHistogramDataPoints(dataPoints pmetric.HistogramDataPointSlice,
	resource pcommon.Resource, settings Settings, baseName string, metadata metadata,
) {
	for x := 0; x < dataPoints.Len(); x++ {
		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		baseLabels := createAttributes(resource, pt.Attributes(), settings.ExternalLabels, nil, false, c.labelNamer)
		noRecordedValue := pt.Flags().NoRecordedValue()

		// If the sum is unset, it indicates the _sum metric point should be
		// omitted
		if pt.HasSum() {
			c.addSampleWithLabels(pt.Sum(), timestamp, noRecordedValue, baseName+sumStr, baseLabels, "", "", metadata)
		}

		// treat count as a sample in an individual TimeSeries
		c.addSampleWithLabels(float64(pt.Count()), timestamp, noRecordedValue, baseName+countStr, baseLabels, "", "", metadata)

		// cumulative count for conversion to cumulative histogram
		var cumulativeCount uint64

		// process each bound, based on histograms proto definition, # of buckets = # of explicit bounds + 1
		for i := 0; i < pt.ExplicitBounds().Len() && i < pt.BucketCounts().Len(); i++ {
			bound := pt.ExplicitBounds().At(i)
			cumulativeCount += pt.BucketCounts().At(i)
			boundStr := strconv.FormatFloat(bound, 'f', -1, 64)
			c.addSampleWithLabels(float64(cumulativeCount), timestamp, noRecordedValue, baseName+bucketStr, baseLabels, leStr, boundStr, metadata)
		}
		// add le=+Inf bucket
		c.addSampleWithLabels(float64(pt.Count()), timestamp, noRecordedValue, baseName+bucketStr, baseLabels, leStr, pInfStr, metadata)

		// TODO implement exemplars support
	}
}

// addExponentialHistogramDataPoints converts exponential histogram data points to Prometheus remote write v2 format.
func (c *prometheusConverterV2) addExponentialHistogramDataPoints(dataPoints pmetric.ExponentialHistogramDataPointSlice,
	resource pcommon.Resource, settings Settings, baseName string, metadata metadata,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		pt := dataPoints.At(x)
		baseLabels := createAttributes(resource, pt.Attributes(), settings.ExternalLabels, nil, false, c.labelNamer)

		histogram, err := exponentialToNativeHistogramV2(pt)
		if err != nil {
			return err
		}

		c.addHistogramWithLabels(histogram, baseName, baseLabels, metadata)

		// TODO implement exemplars support for v2
	}

	return nil
}

// exponentialToNativeHistogramV2 translates OTel Exponential Histogram data point
// to Prometheus Native Histogram for v2 format.
func exponentialToNativeHistogramV2(p pmetric.ExponentialHistogramDataPoint) (writev2.Histogram, error) {
	scale := p.Scale()
	if scale < -4 {
		return writev2.Histogram{},
			fmt.Errorf("cannot convert exponential to native histogram."+
				" Scale must be >= -4, was %d", scale)
	}

	var scaleDown int32
	if scale > 8 {
		scaleDown = scale - 8
		scale = 8
	}

	pSpans, pDeltas := convertBucketsLayout(p.Positive(), scaleDown)
	nSpans, nDeltas := convertBucketsLayout(p.Negative(), scaleDown)

	// Convert prompb spans to writev2 spans
	positiveSpans := make([]writev2.BucketSpan, len(pSpans))
	for i, span := range pSpans {
		positiveSpans[i] = writev2.BucketSpan{
			Offset: span.Offset,
			Length: span.Length,
		}
	}

	negativeSpans := make([]writev2.BucketSpan, len(nSpans))
	for i, span := range nSpans {
		negativeSpans[i] = writev2.BucketSpan{
			Offset: span.Offset,
			Length: span.Length,
		}
	}

	h := writev2.Histogram{
		// The counter reset detection must be compatible with Prometheus to
		// safely set ResetHint to NO. This is not ensured currently.
		// Sending a sample that triggers counter reset but with ResetHint==NO
		// would lead to Prometheus panic as it does not double check the hint.
		// Thus we're explicitly saying UNKNOWN here, which is always safe.
		ResetHint: writev2.Histogram_RESET_HINT_UNSPECIFIED,
		Schema:    scale,

		ZeroCount:     &writev2.Histogram_ZeroCountInt{ZeroCountInt: p.ZeroCount()},
		ZeroThreshold: defaultZeroThreshold,

		PositiveSpans:  positiveSpans,
		PositiveDeltas: pDeltas,
		NegativeSpans:  negativeSpans,
		NegativeDeltas: nDeltas,

		Timestamp: convertTimeStamp(p.Timestamp()),
	}

	if p.Flags().NoRecordedValue() {
		h.Sum = math.Float64frombits(value.StaleNaN)
		h.Count = &writev2.Histogram_CountInt{CountInt: value.StaleNaN}
	} else {
		if p.HasSum() {
			h.Sum = p.Sum()
		}
		h.Count = &writev2.Histogram_CountInt{CountInt: p.Count()}
	}

	return h, nil
}

// addHistogramWithLabels adds a histogram sample with the given labels to the converter.
func (c *prometheusConverterV2) addHistogramWithLabels(histogram writev2.Histogram, baseName string, baseLabels []prompb.Label, metadata metadata) {
	// Create labels for the histogram metric
	labels := make([]prompb.Label, len(baseLabels)+1)
	copy(labels, baseLabels)
	labels[len(baseLabels)] = prompb.Label{
		Name:  model.MetricNameLabel,
		Value: baseName,
	}

	// Sort labels
	sort.Slice(labels, func(i, j int) bool {
		return labels[i].Name < labels[j].Name
	})

	// Convert labels to symbol refs
	buf := make([]uint32, 0, len(labels)*2)
	for _, l := range labels {
		nameRef := c.symbolTable.Symbolize(l.Name)
		valueRef := c.symbolTable.Symbolize(l.Value)
		buf = append(buf, nameRef, valueRef)
	}

	ts := &writev2.TimeSeries{
		LabelsRefs: buf,
		Histograms: []writev2.Histogram{histogram},
		Metadata: writev2.Metadata{
			Type:    metadata.Type,
			HelpRef: c.symbolTable.Symbolize(metadata.Help),
			UnitRef: c.symbolTable.Symbolize(metadata.Unit),
		},
	}

	c.unique[timeSeriesSignature(labels)] = ts
}
