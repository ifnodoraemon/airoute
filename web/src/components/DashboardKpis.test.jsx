import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import DashboardKpis from './DashboardKpis';

const defaults = {
  currentRPM: 0, currentTPM: 0, fmtPct: value => value,
  isZh: true, percentiles: { sampled: 0 }, slaAvailability: '—', tokenMetrics: {},
};

describe('DashboardKpis spend windows', () => {
  it('shows server calendar spend with explicit UTC boundaries', () => {
    render(<DashboardKpis {...defaults} stats={{ total_cost: 9, today_cost: 1.25, month_cost: 8.5 }} />);
    expect(screen.getByText('今日 (UTC): ¥1.2500 · 本月 (UTC): ¥8.5000')).toBeInTheDocument();
    expect(screen.getByText('¥9.0000')).toBeInTheDocument();
  });

  it('warns only about a material measured increase, never a missing baseline', () => {
    const { rerender } = render(<DashboardKpis {...defaults} stats={{ cost_24h: 3, previous_cost_24h: 1 }} />);
    expect(screen.getByRole('status')).toHaveTextContent('超过前 24 小时的 2 倍');
    rerender(<DashboardKpis {...defaults} stats={{ cost_24h: 3, previous_cost_24h: 0 }} />);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    rerender(<DashboardKpis {...defaults} stats={{ cost_24h: 0.03, previous_cost_24h: 0.01 }} />);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
