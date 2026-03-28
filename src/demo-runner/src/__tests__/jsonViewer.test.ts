import { describe, it, expect } from 'vitest';
import { renderValue } from '../components/JsonViewer';

describe('renderValue', () => {
  it('renders null', () => {
    const result = renderValue(null, 0);
    expect(result).toBeTruthy();
  });

  it('renders undefined', () => {
    const result = renderValue(undefined, 0);
    expect(result).toBeTruthy();
  });

  it('renders string', () => {
    const result = renderValue('hello', 0);
    expect(result).toBeTruthy();
  });

  it('renders number', () => {
    const result = renderValue(42, 0);
    expect(result).toBeTruthy();
  });

  it('renders boolean', () => {
    const result = renderValue(true, 0);
    expect(result).toBeTruthy();
  });

  it('renders empty array', () => {
    const result = renderValue([], 0);
    expect(result).toBeTruthy();
  });

  it('renders array with items', () => {
    const result = renderValue([1, 'two', null], 0);
    expect(result).toBeTruthy();
  });

  it('renders empty object', () => {
    const result = renderValue({}, 0);
    expect(result).toBeTruthy();
  });

  it('renders object with nested values', () => {
    const result = renderValue({ key: 'value', num: 42, nested: { inner: true } }, 0);
    expect(result).toBeTruthy();
  });
});
