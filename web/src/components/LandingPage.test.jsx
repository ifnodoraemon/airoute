import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import LandingPage from './LandingPage';

// The landing page is the first thing an anonymous visitor sees and the only screen
// that has to work without a session. Its one real branch is whether the visitor is
// signed in: a signed-in one is offered the console, an anonymous one the login.

function setup(props = {}) {
  const handlers = {
    onOpenLogin: vi.fn(),
    onEnterConsole: vi.fn(),
    onViewDocs: vi.fn(),
    onViewStatus: vi.fn(),
    setLang: vi.fn(),
    toggleLang: vi.fn(),
    showToast: vi.fn(),
  };
  const utils = render(
    <LandingPage
      isLoggedIn={false}
      adminUser={null}
      stats={{}}
      channelsCount={0}
      modelsCount={0}
      lang="zh"
      {...handlers}
      {...props}
    />
  );
  return { ...utils, ...handlers };
}

describe('LandingPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders the public page', () => {
    setup();

    expect(screen.getAllByText('Airoute').length).toBeGreaterThan(0);
  });

  describe('the session branch', () => {
    it('offers login to an anonymous visitor', () => {
      setup({ isLoggedIn: false });

      expect(screen.getAllByText(/登录 \/ 注册|Sign In \/ Register/).length).toBeGreaterThan(0);
      expect(screen.queryByText(/进入工作台|Console →/)).not.toBeInTheDocument();
    });

    it('offers the console to a signed-in visitor', () => {
      setup({ isLoggedIn: true, adminUser: { username: 'root', role: 'admin' } });

      expect(screen.getAllByText(/进入工作台|Console →/).length).toBeGreaterThan(0);
      expect(screen.queryByText(/登录 \/ 注册|Sign In \/ Register/)).not.toBeInTheDocument();
    });

    it('reports the console entry', () => {
      const { onEnterConsole } = setup({ isLoggedIn: true, adminUser: { username: 'root' } });

      fireEvent.click(screen.getAllByText(/进入工作台|Console →/)[0]);

      expect(onEnterConsole).toHaveBeenCalledTimes(1);
    });

    it('reports the login request', () => {
      const { onOpenLogin } = setup({ isLoggedIn: false });

      fireEvent.click(screen.getAllByText(/登录 \/ 注册|Sign In \/ Register/)[0]);

      expect(onOpenLogin).toHaveBeenCalledTimes(1);
    });
  });

  describe('code snippets', () => {
    it('starts on the Python tab', () => {
      setup();

      // The Python snippet contains the client construction.
      expect(screen.getByText(/from openai import OpenAI/)).toBeInTheDocument();
    });

    it('switches to the curl tab', () => {
      setup();

      fireEvent.click(screen.getByRole('button', { name: /cURL|curl/i }));

      expect(screen.getByText(/curl -X POST|curl https/)).toBeInTheDocument();
    });
  });

  it('renders in English when asked', () => {
    setup({ lang: 'en' });

    expect(screen.getAllByText('Airoute').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/Sign In \/ Register/).length).toBeGreaterThan(0);
  });
});
