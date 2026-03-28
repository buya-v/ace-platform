import React, { createContext, useContext, useReducer, ReactNode } from 'react';
import { StepResult } from '../types/step';

export interface DemoState {
  gatewayUrl: string;
  activeSectionId: string;
  results: Record<string, StepResult>;
  appState: Record<string, unknown>;
  runningStepId: string | null;
  runAllInProgress: boolean;
  checkedItems: Record<string, boolean>;
}

export type DemoAction =
  | { type: 'SET_GATEWAY_URL'; url: string }
  | { type: 'SET_ACTIVE_SECTION'; sectionId: string }
  | { type: 'SET_STEP_RUNNING'; stepId: string }
  | { type: 'SET_STEP_RESULT'; result: StepResult; newState?: Record<string, unknown> }
  | { type: 'RESET_ALL' }
  | { type: 'SET_RUN_ALL'; inProgress: boolean }
  | { type: 'TOGGLE_CHECKLIST_ITEM'; itemId: string };

export const initialState: DemoState = {
  gatewayUrl: 'http://localhost:8080',
  activeSectionId: 'env-setup',
  results: {},
  appState: {},
  runningStepId: null,
  runAllInProgress: false,
  checkedItems: {},
};

export function demoReducer(state: DemoState, action: DemoAction): DemoState {
  switch (action.type) {
    case 'SET_GATEWAY_URL':
      return { ...state, gatewayUrl: action.url };
    case 'SET_ACTIVE_SECTION':
      return { ...state, activeSectionId: action.sectionId };
    case 'SET_STEP_RUNNING':
      return {
        ...state,
        runningStepId: action.stepId,
        results: {
          ...state.results,
          [action.stepId]: {
            stepId: action.stepId,
            status: 'RUNNING',
            requestMethod: '',
            requestUrl: '',
            requestHeaders: {},
            requestBody: null,
            responseStatus: null,
            responseBody: null,
            responseTime: 0,
          },
        },
      };
    case 'SET_STEP_RESULT':
      return {
        ...state,
        runningStepId: null,
        results: { ...state.results, [action.result.stepId]: action.result },
        appState: action.newState ?? state.appState,
      };
    case 'RESET_ALL':
      return { ...initialState, gatewayUrl: state.gatewayUrl, activeSectionId: state.activeSectionId };
    case 'SET_RUN_ALL':
      return { ...state, runAllInProgress: action.inProgress };
    case 'TOGGLE_CHECKLIST_ITEM':
      return {
        ...state,
        checkedItems: {
          ...state.checkedItems,
          [action.itemId]: !state.checkedItems[action.itemId],
        },
      };
    default:
      return state;
  }
}

interface DemoContextValue {
  state: DemoState;
  dispatch: React.Dispatch<DemoAction>;
}

const DemoContext = createContext<DemoContextValue | null>(null);

export function DemoProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(demoReducer, initialState);
  return <DemoContext.Provider value={{ state, dispatch }}>{children}</DemoContext.Provider>;
}

export function useDemo(): DemoContextValue {
  const ctx = useContext(DemoContext);
  if (!ctx) throw new Error('useDemo must be used within DemoProvider');
  return ctx;
}
