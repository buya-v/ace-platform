import { useCallback } from 'react';
import { useDemo } from '../contexts/DemoContext';
import { allSections } from '../data/sections';
import { isChecklistSection } from '../types/section';
import { executeStep } from '../services/executor';

export function useDemoRunner() {
  const { state, dispatch } = useDemo();

  const runAll = useCallback(async () => {
    dispatch({ type: 'SET_RUN_ALL', inProgress: true });
    let currentState = { ...state.appState };

    for (const section of allSections) {
      if (isChecklistSection(section)) continue;
      for (const step of section.steps) {
        dispatch({ type: 'SET_STEP_RUNNING', stepId: step.id });
        const { result, newState } = await executeStep(step, state.gatewayUrl, currentState);
        currentState = newState;
        dispatch({ type: 'SET_STEP_RESULT', result, newState });
      }
    }

    dispatch({ type: 'SET_RUN_ALL', inProgress: false });
  }, [state.gatewayUrl, state.appState, dispatch]);

  const reset = useCallback(() => {
    dispatch({ type: 'RESET_ALL' });
  }, [dispatch]);

  const passCount = Object.values(state.results).filter((r) => r.status === 'PASS').length;
  const failCount = Object.values(state.results).filter((r) => r.status === 'FAIL').length;

  return { runAll, reset, passCount, failCount, runAllInProgress: state.runAllInProgress };
}
