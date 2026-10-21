import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import PlaygroundView from './PlaygroundView';

// Regression guard for a crash that reached the browser:
//
//   Uncaught ReferenceError: Cannot access 'j' before initialization
//     at PlaygroundView-<hash>.js:1:625
//
// The off-peak footer derived `activeChatModel` from `playModel`, but the block
// sat above `const [playModel, setPlayModel] = useState(...)`. Reading a `const`
// binding inside its temporal dead zone throws, and because the block runs on
// every render, the whole playground route was a blank page. Nothing in the suite
// rendered this component, and `no-undef` cannot see the problem: `playModel` is
// declared, just too late.
//
// Mounting the view is therefore the actual assertion — the broken version throws
// before anything reaches the DOM.

const baseProps = {
  keys: [{ key: 'sk-test', allowed_models: ['*'] }],
  models: ['gpt-4o', 'claude-3-5-sonnet'],
  modelRoutes: [],
  prices: [],
  lang: 'zh',
};

describe('PlaygroundView', () => {
  it('mounts without a temporal-dead-zone ReferenceError', () => {
    render(<PlaygroundView {...baseProps} />);

    // The footer renders the "no discount" branch with empty pricing data.
    expect(screen.getAllByText('该模型未配置闲时折扣').length).toBeGreaterThan(0);
  });

  it('resolves the off-peak discount of the selected model', () => {
    render(
      <PlaygroundView
        {...baseProps}
        initialModel="gpt-4o"
        prices={[
          { model: 'gpt-4o', group_name: 'default', off_peak_enabled: true, off_peak_discount: 0.5 },
        ]}
      />
    );

    expect(screen.getAllByText('闲时折扣已配置 (5 折, 按时段生效)').length).toBeGreaterThan(0);
  });

  it('starts on the model passed through initialModel', () => {
    render(<PlaygroundView {...baseProps} initialModel="claude-3-5-sonnet" />);

    expect(screen.getByDisplayValue('claude-3-5-sonnet')).toBeInTheDocument();
  });
});
