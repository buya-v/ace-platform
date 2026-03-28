import { describe, it, expect } from 'vitest';
import { demoReducer, initialState, DemoState } from '../contexts/DemoContext';
import { StepResult } from '../types/step';

describe('demoReducer', () => {
  it('sets gateway URL', () => {
    const state = demoReducer(initialState, { type: 'SET_GATEWAY_URL', url: 'http://example.com' });
    expect(state.gatewayUrl).toBe('http://example.com');
  });

  it('sets active section', () => {
    const state = demoReducer(initialState, { type: 'SET_ACTIVE_SECTION', sectionId: 'trading' });
    expect(state.activeSectionId).toBe('trading');
  });

  it('sets step running', () => {
    const state = demoReducer(initialState, { type: 'SET_STEP_RUNNING', stepId: 'env-1' });
    expect(state.runningStepId).toBe('env-1');
    expect(state.results['env-1'].status).toBe('RUNNING');
  });

  it('sets step result and clears running', () => {
    const running: DemoState = { ...initialState, runningStepId: 'env-1' };
    const result: StepResult = {
      stepId: 'env-1',
      status: 'PASS',
      requestMethod: 'GET',
      requestUrl: '/healthz',
      requestHeaders: {},
      requestBody: null,
      responseStatus: 200,
      responseBody: { status: 'ok' },
      responseTime: 42,
    };
    const state = demoReducer(running, { type: 'SET_STEP_RESULT', result, newState: { token: 'abc' } });
    expect(state.runningStepId).toBeNull();
    expect(state.results['env-1'].status).toBe('PASS');
    expect(state.appState.token).toBe('abc');
  });

  it('preserves appState when newState is not provided', () => {
    const prev: DemoState = { ...initialState, appState: { existing: 'value' } };
    const result: StepResult = {
      stepId: 'env-1',
      status: 'PASS',
      requestMethod: 'GET',
      requestUrl: '/healthz',
      requestHeaders: {},
      requestBody: null,
      responseStatus: 200,
      responseBody: null,
      responseTime: 10,
    };
    const state = demoReducer(prev, { type: 'SET_STEP_RESULT', result });
    expect(state.appState.existing).toBe('value');
  });

  it('resets all state but preserves gateway URL and section', () => {
    const modified: DemoState = {
      ...initialState,
      gatewayUrl: 'http://custom:9090',
      activeSectionId: 'trading',
      results: { 'env-1': {} as StepResult },
      appState: { token: 'abc' },
    };
    const state = demoReducer(modified, { type: 'RESET_ALL' });
    expect(state.gatewayUrl).toBe('http://custom:9090');
    expect(state.activeSectionId).toBe('trading');
    expect(state.results).toEqual({});
    expect(state.appState).toEqual({});
  });

  it('sets run all in progress', () => {
    const state = demoReducer(initialState, { type: 'SET_RUN_ALL', inProgress: true });
    expect(state.runAllInProgress).toBe(true);
  });

  it('toggles checklist item', () => {
    const state1 = demoReducer(initialState, { type: 'TOGGLE_CHECKLIST_ITEM', itemId: 'sec-1' });
    expect(state1.checkedItems['sec-1']).toBe(true);
    const state2 = demoReducer(state1, { type: 'TOGGLE_CHECKLIST_ITEM', itemId: 'sec-1' });
    expect(state2.checkedItems['sec-1']).toBe(false);
  });

  it('returns state for unknown action', () => {
    const state = demoReducer(initialState, { type: 'UNKNOWN' } as never);
    expect(state).toBe(initialState);
  });
});
