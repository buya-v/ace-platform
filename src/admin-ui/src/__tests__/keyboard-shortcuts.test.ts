import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useKeyboardShortcuts, KeyboardShortcutConfig } from '../hooks/useKeyboardShortcuts';

function fireKey(key: string, opts: Partial<KeyboardEventInit> = {}) {
  const event = new KeyboardEvent('keydown', {
    key,
    bubbles: true,
    cancelable: true,
    ...opts,
  });
  // spy on preventDefault
  const pd = vi.spyOn(event, 'preventDefault');
  document.dispatchEvent(event);
  return { event, preventDefault: pd };
}

function setup(overrides: Partial<KeyboardShortcutConfig> = {}) {
  const config: KeyboardShortcutConfig = {
    navigate: vi.fn(),
    onRefresh: vi.fn(),
    onExport: vi.fn(),
    onEscape: vi.fn(),
    ...overrides,
  };
  const result = renderHook(() => useKeyboardShortcuts(config));
  return { config, result };
}

describe('useKeyboardShortcuts', () => {
  describe('Ctrl+1 through Ctrl+9 navigation', () => {
    const routes: [string, string][] = [
      ['1', '/dashboard'],
      ['2', '/dashboard/monitoring'],
      ['3', '/dashboard/orderbook'],
      ['4', '/dashboard/positions'],
      ['5', '/dashboard/risk'],
      ['6', '/dashboard/margin'],
      ['7', '/dashboard/settlement'],
      ['8', '/dashboard/circuit-breakers'],
      ['9', '/dashboard/warehouse'],
    ];

    it.each(routes)('Ctrl+%s navigates to %s', (key, path) => {
      const { config } = setup();
      const { preventDefault } = fireKey(key, { ctrlKey: true });
      expect(config.navigate).toHaveBeenCalledWith(path);
      expect(preventDefault).toHaveBeenCalled();
    });

    it.each(routes)('Meta+%s navigates to %s', (key, path) => {
      const { config } = setup();
      fireKey(key, { metaKey: true });
      expect(config.navigate).toHaveBeenCalledWith(path);
    });
  });

  describe('Ctrl+R refresh', () => {
    it('calls onRefresh and prevents default', () => {
      const { config } = setup();
      const { preventDefault } = fireKey('r', { ctrlKey: true });
      expect(config.onRefresh).toHaveBeenCalled();
      expect(preventDefault).toHaveBeenCalled();
    });

    it('works with uppercase R', () => {
      const { config } = setup();
      fireKey('R', { ctrlKey: true });
      expect(config.onRefresh).toHaveBeenCalled();
    });
  });

  describe('Ctrl+E export', () => {
    it('calls onExport and prevents default', () => {
      const { config } = setup();
      const { preventDefault } = fireKey('e', { ctrlKey: true });
      expect(config.onExport).toHaveBeenCalled();
      expect(preventDefault).toHaveBeenCalled();
    });

    it('works with uppercase E', () => {
      const { config } = setup();
      fireKey('E', { metaKey: true });
      expect(config.onExport).toHaveBeenCalled();
    });
  });

  describe('Escape', () => {
    it('calls onEscape', () => {
      const { config } = setup();
      fireKey('Escape');
      expect(config.onEscape).toHaveBeenCalled();
    });

    it('closes help overlay when open', () => {
      const { config, result } = setup();
      // Open help first
      act(() => { fireKey('?'); });
      expect(result.result.current.showHelp).toBe(true);
      // Press Escape
      act(() => { fireKey('Escape'); });
      expect(result.result.current.showHelp).toBe(false);
      expect(config.onEscape).toHaveBeenCalled();
    });
  });

  describe('? toggles help', () => {
    it('toggles showHelp on', () => {
      const { result } = setup();
      expect(result.result.current.showHelp).toBe(false);
      act(() => { fireKey('?'); });
      expect(result.result.current.showHelp).toBe(true);
    });

    it('toggles showHelp off', () => {
      const { result } = setup();
      act(() => { fireKey('?'); });
      expect(result.result.current.showHelp).toBe(true);
      act(() => { fireKey('?'); });
      expect(result.result.current.showHelp).toBe(false);
    });

    it('does not toggle when Ctrl is held', () => {
      const { result } = setup();
      act(() => { fireKey('?', { ctrlKey: true }); });
      expect(result.result.current.showHelp).toBe(false);
    });
  });

  describe('input element suppression', () => {
    it('ignores shortcuts when target is an INPUT', () => {
      const { config } = setup();
      const input = document.createElement('input');
      document.body.appendChild(input);
      const event = new KeyboardEvent('keydown', {
        key: '1',
        ctrlKey: true,
        bubbles: true,
      });
      Object.defineProperty(event, 'target', { value: input });
      document.dispatchEvent(event);
      expect(config.navigate).not.toHaveBeenCalled();
      document.body.removeChild(input);
    });

    it('ignores shortcuts when target is a TEXTAREA', () => {
      const { config } = setup();
      const textarea = document.createElement('textarea');
      document.body.appendChild(textarea);
      const event = new KeyboardEvent('keydown', {
        key: 'r',
        ctrlKey: true,
        bubbles: true,
      });
      Object.defineProperty(event, 'target', { value: textarea });
      document.dispatchEvent(event);
      expect(config.onRefresh).not.toHaveBeenCalled();
      document.body.removeChild(textarea);
    });

    it('ignores shortcuts when target is a SELECT', () => {
      const { config } = setup();
      const select = document.createElement('select');
      document.body.appendChild(select);
      const event = new KeyboardEvent('keydown', {
        key: 'e',
        ctrlKey: true,
        bubbles: true,
      });
      Object.defineProperty(event, 'target', { value: select });
      document.dispatchEvent(event);
      expect(config.onExport).not.toHaveBeenCalled();
      document.body.removeChild(select);
    });
  });

  describe('setShowHelp', () => {
    it('allows manual control of showHelp', () => {
      const { result } = setup();
      act(() => { result.result.current.setShowHelp(true); });
      expect(result.result.current.showHelp).toBe(true);
      act(() => { result.result.current.setShowHelp(false); });
      expect(result.result.current.showHelp).toBe(false);
    });
  });

  describe('cleanup', () => {
    it('removes event listener on unmount', () => {
      const removeSpy = vi.spyOn(document, 'removeEventListener');
      const { result } = setup();
      result.unmount();
      expect(removeSpy).toHaveBeenCalledWith('keydown', expect.any(Function));
      removeSpy.mockRestore();
    });
  });

  describe('no-op callbacks', () => {
    it('works without optional callbacks', () => {
      const { config } = setup({ onRefresh: undefined, onExport: undefined, onEscape: undefined });
      // Should not throw
      fireKey('r', { ctrlKey: true });
      fireKey('e', { ctrlKey: true });
      fireKey('Escape');
    });
  });

  describe('non-matching keys ignored', () => {
    it('does not navigate for Ctrl+0', () => {
      const { config } = setup();
      fireKey('0', { ctrlKey: true });
      expect(config.navigate).not.toHaveBeenCalled();
    });

    it('does not navigate for plain number keys', () => {
      const { config } = setup();
      fireKey('1');
      expect(config.navigate).not.toHaveBeenCalled();
    });
  });
});
