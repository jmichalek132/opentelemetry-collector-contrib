// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusremotewriteexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"

import (
	"context"
	"math"
	"sync"

	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.uber.org/multierr"
)

// Request represents a generic request that can be marshaled
type Request interface {
	Marshal() ([]byte, error)
	Size() int
}

// exportRequests handles the common logic for exporting requests with concurrency control.
// This eliminates duplication between v1 and v2 export functions.
func (prwe *prwExporter) exportRequests(ctx context.Context, requests []Request) error {
	input := make(chan Request, len(requests))
	for _, request := range requests {
		input <- request
	}
	close(input)

	var wg sync.WaitGroup

	concurrencyLimit := int(math.Min(float64(prwe.concurrency), float64(len(requests))))
	wg.Add(concurrencyLimit) // used to wait for workers to be finished

	var mu sync.Mutex
	var errs error
	// Run concurrencyLimit of workers until there
	// is no more requests to execute in the input channel.
	for i := 0; i < concurrencyLimit; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done(): // Check firstly to ensure that the context wasn't cancelled.
					return

				case request, ok := <-input:
					if !ok {
						return
					}

					buf := bufferPool.Get().(*buffer)
					buf.protobuf.Reset()

					data, errMarshal := request.Marshal()
					if errMarshal != nil {
						mu.Lock()
						errs = multierr.Append(errs, consumererror.NewPermanent(errMarshal))
						mu.Unlock()
						bufferPool.Put(buf)
						return
					}

					// Set the marshaled data to the protobuf buffer
					buf.protobuf.SetBuf(data)

					if errExecute := prwe.execute(ctx, buf); errExecute != nil {
						mu.Lock()
						errs = multierr.Append(errs, consumererror.NewPermanent(errExecute))
						mu.Unlock()
					}
					bufferPool.Put(buf)
				}
			}
		}()
	}
	wg.Wait()

	return errs
}
