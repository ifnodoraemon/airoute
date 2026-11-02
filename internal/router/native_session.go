package router

import (
	"context"
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func (d *Dispatcher) BeginNativeSession(ctx context.Context, ch *model.ChannelConfig, name string) (context.Context, context.CancelFunc, error) {
	if !model.IsGeminiProvider(ch.Type) {
		return nil, nil, fmt.Errorf("native Google channel required")
	}
	if !d.circuitBreaker.CanExecute(ch.Name) {
		return nil, nil, fmt.Errorf("native upstream circuit is open")
	}
	call, cancel, err := d.beginChannelCall(ctx, ch, name)
	if err != nil {
		d.circuitBreaker.AbortCanary(ch.Name)
	}
	return call, cancel, err
}

func (d *Dispatcher) ObserveNativeSession(ctx context.Context, ch *model.ChannelConfig, name string, err error) {
	if err != nil {
		d.observeUpstreamError(ctx, ch, name, err)
		d.circuitBreaker.AbortCanary(ch.Name)
		return
	}
	d.circuitBreaker.RecordSuccess(ch.Name)
	d.rememberSessionChannel(ctx, name, ch.Name)
}
