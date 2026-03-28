import { useCallback } from 'react';
import { useDemo } from '../contexts/DemoContext';
import { StepDefinition } from '../types/step';
import { executeStep } from '../services/executor';

export function useStepExecutor() {
  const { state, dispatch } = useDemo();

  const runStep = useCallback(
    async (step: StepDefinition) => {
      dispatch({ type: 'SET_STEP_RUNNING', stepId: step.id });
      const { result, newState } = await executeStep(step, state.gatewayUrl, state.appState);
      dispatch({ type: 'SET_STEP_RESULT', result, newState });
      return result;
    },
    [state.gatewayUrl, state.appState, dispatch],
  );

  return { runStep, runningStepId: state.runningStepId };
}
