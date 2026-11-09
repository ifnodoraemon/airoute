package router

import (
	"context"
	"errors"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

type unaryAttemptState struct {
	dispatcher         *Dispatcher
	ctx                context.Context
	model              string
	estimate           func() int
	attempted, skipped []string
	lastErr            error
}

func (s *unaryAttemptState) available(ch *model.ChannelConfig, attempt int) (bool, error) {
	if err := s.ctx.Err(); err != nil {
		if requestBudgetExceeded(s.ctx, err) {
			return false, newRequestBudgetError(s.model, s.attempted, s.lastErr)
		}
		return false, newCanceledError(s.ctx, s.attempted, "", 0)
	}
	if s.dispatcher.circuitBreaker.CanExecute(ch.Name) {
		return true, nil
	}
	telemetry.WarnLogger(routerLog(s.ctx), "breaker-open:"+ch.Name, 30*time.Second,
		"circuit breaker is OPEN, bypassing dead upstream", "channel", ch.Name, "attempt", attempt)
	s.skipped = append(s.skipped, ch.Name)
	return false, nil
}

func (s *unaryAttemptState) unsupported(ch *model.ChannelConfig, err error) {
	s.lastErr = err
	s.dispatcher.circuitBreaker.AbortCanary(ch.Name)
}

func (s *unaryAttemptState) admissionFailure(ch *model.ChannelConfig, err error) error {
	s.dispatcher.circuitBreaker.AbortCanary(ch.Name)
	if s.ctx.Err() != nil {
		return admissionDispatchError(s.ctx, s.model, s.attempted, err)
	}
	s.lastErr = err
	s.skipped = append(s.skipped, ch.Name)
	return nil
}

func (s *unaryAttemptState) failure(ch *model.ChannelConfig, err error, attempt int) error {
	d := s.dispatcher
	if s.ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
		d.circuitBreaker.AbortCanary(ch.Name)
		return newCanceledError(s.ctx, s.attempted, ch.Name, s.estimate())
	}
	if ctxErr := s.ctx.Err(); ctxErr != nil {
		d.circuitBreaker.AbortCanary(ch.Name)
		if requestBudgetExceeded(s.ctx, ctxErr) {
			return newRequestBudgetError(s.model, append(s.attempted, ch.Name), err)
		}
		return newCanceledError(s.ctx, s.attempted, ch.Name, s.estimate())
	}
	s.lastErr = err
	d.observeUpstreamError(s.ctx, ch, s.model, err)
	switch {
	case provider.IsModelNotFound(err):
		d.circuitBreaker.AbortCanary(ch.Name)
	case provider.IsFatalPayloadError(err):
		d.circuitBreaker.AbortCanary(ch.Name)
		return err
	case !provider.IsChannelFailure(err):
		d.circuitBreaker.AbortCanary(ch.Name)
	default:
		s.attempted = append(s.attempted, d.recordChannelFailure(s.ctx, ch.Name, err, attempt))
		return nil
	}
	s.attempted = append(s.attempted, ch.Name)
	return nil
}
