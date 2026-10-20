import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import MarkdownRenderer from './MarkdownRenderer';

// MarkdownRenderer turns model output into DOM, and model output is untrusted text.
// The part that matters most is the URL filter: a link or image whose target is a
// javascript: URL is an injection, and the renderer is the only thing standing
// between the two.

describe('MarkdownRenderer', () => {
  it('renders nothing for empty content', () => {
    const { container } = render(<MarkdownRenderer content="" />);

    expect(container).toBeEmptyDOMElement();
  });

  it('renders plain text', () => {
    render(<MarkdownRenderer content="the gateway is healthy" />);

    expect(screen.getByText(/the gateway is healthy/)).toBeInTheDocument();
  });

  it('renders headings and lists', () => {
    render(<MarkdownRenderer content={'# Title\n\n- one\n- two'} />);

    expect(screen.getByText('Title')).toBeInTheDocument();
    expect(screen.getByText('one')).toBeInTheDocument();
    expect(screen.getByText('two')).toBeInTheDocument();
  });

  it('renders a table', () => {
    render(<MarkdownRenderer content={'| a | b |\n| --- | --- |\n| 1 | 2 |'} />);

    expect(screen.getByText('a')).toBeInTheDocument();
    expect(screen.getByText('1')).toBeInTheDocument();
  });

  describe('link safety', () => {
    it('keeps an https link', () => {
      render(<MarkdownRenderer content="[docs](https://example.com/guide)" />);

      const link = screen.getByRole('link', { name: 'docs' });
      expect(link).toHaveAttribute('href', 'https://example.com/guide');
    });

    it('keeps an http link', () => {
      render(<MarkdownRenderer content="[docs](http://example.com)" />);

      expect(screen.getByRole('link', { name: 'docs' })).toHaveAttribute('href', 'http://example.com');
    });

    it('keeps an in-page anchor', () => {
      render(<MarkdownRenderer content="[jump](#section)" />);

      expect(screen.getByRole('link', { name: 'jump' })).toHaveAttribute('href', '#section');
    });

    it('keeps a mailto link', () => {
      render(<MarkdownRenderer content="[mail](mailto:ops@example.com)" />);

      expect(screen.getByRole('link', { name: 'mail' })).toHaveAttribute('href', 'mailto:ops@example.com');
    });

    it('refuses a javascript: URL', () => {
      // The classic injection: a link that executes when clicked.
      render(<MarkdownRenderer content="[click](javascript:alert(1))" />);

      expect(screen.queryByRole('link')).not.toBeInTheDocument();
    });

    it('refuses a data: URL on a link', () => {
      render(<MarkdownRenderer content="[click](data:text/html;base64,PHNjcmlwdD4=)" />);

      expect(screen.queryByRole('link')).not.toBeInTheDocument();
    });

    it('refuses a vbscript: URL', () => {
      render(<MarkdownRenderer content="[click](vbscript:msgbox)" />);

      expect(screen.queryByRole('link')).not.toBeInTheDocument();
    });
  });

  describe('image safety', () => {
    it('keeps an https image', () => {
      render(<MarkdownRenderer content="![chart](https://example.com/chart.png)" />);

      expect(screen.getByRole('img', { name: 'chart' })).toHaveAttribute('src', 'https://example.com/chart.png');
    });

    it('keeps an inline base64 image', () => {
      // The chart renderer embeds images this way.
      render(<MarkdownRenderer content="![chart](data:image/png;base64,iVBORw0KGgo=)" />);

      expect(screen.getByRole('img', { name: 'chart' })).toHaveAttribute('src', 'data:image/png;base64,iVBORw0KGgo=');
    });

    it('refuses a data: URL that is not an image', () => {
      // data:text/html is a document, not a picture, and can carry script.
      render(<MarkdownRenderer content="![x](data:text/html;base64,PHNjcmlwdD4=)" />);

      expect(screen.queryByRole('img')).not.toBeInTheDocument();
    });

    it('refuses a javascript: image source', () => {
      render(<MarkdownRenderer content="![x](javascript:alert(1))" />);

      expect(screen.queryByRole('img')).not.toBeInTheDocument();
    });
  });

  describe('streaming', () => {
    it('closes a code fence that was cut off mid-stream', () => {
      // A streaming answer arrives in pieces; the closing fence is often not in the
      // text yet, and without this the rest of the message renders as code.
      const { container } = render(<MarkdownRenderer content={'before\n\n```go\nfunc main() {}'} />);

      expect(container.textContent).toContain('func main() {}');
      expect(container.querySelector('pre, code')).not.toBeNull();
    });

    it('does not add a blank line to the code it closes', () => {
      // The stream is cut mid-answer: a fence is open and prose follows it. Closing
      // the fence by appending "\n```" put an extra blank line inside the block, so
      // the block's own counter said 4 lines for the 3 the stream had produced.
      const { container } = render(
        <MarkdownRenderer content={'intro\n\n```go\ncode\n\ntrailing prose\n'} />
      );

      expect(container.textContent).toContain('3 lines');
      expect(container.textContent).not.toContain('4 lines');
    });

    it('leaves balanced fences alone', () => {
      const { container } = render(<MarkdownRenderer content={'```go\nfunc main() {}\n```\n\nafter'} />);

      expect(container.textContent).toContain('func main() {}');
      expect(container.textContent).toContain('after');
    });
  });

  it('renders a code block with its content intact', () => {
    const { container } = render(<MarkdownRenderer content={'```json\n{"a": 1}\n```'} />);

    expect(container.textContent).toContain('{"a": 1}');
  });

  it('does not execute markup embedded in the text', () => {
    // The renderer builds React elements, so a script tag in the source must be
    // escaped text rather than a live element.
    const { container } = render(<MarkdownRenderer content={'<script>window.pwned = true</script>'} />);

    expect(container.querySelector('script')).toBeNull();
    expect(container.textContent).toContain('window.pwned');
  });
});
