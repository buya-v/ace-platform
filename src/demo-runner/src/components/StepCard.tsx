import React from 'react';
import { StepDefinition } from '../types/step';
import { useDemo } from '../contexts/DemoContext';
import { useStepExecutor } from '../hooks/useStepExecutor';
import { StatusBadge } from './StatusBadge';
import { RequestPanel } from './RequestPanel';
import { ResponsePanel } from './ResponsePanel';
import styles from './StepCard.module.css';

interface StepCardProps {
  step: StepDefinition;
}

export function StepCard({ step }: StepCardProps) {
  const { state } = useDemo();
  const { runStep, runningStepId } = useStepExecutor();
  const result = state.results[step.id];
  const status = result?.status ?? 'PENDING';
  const isRunning = runningStepId === step.id;

  const resolvedUrl = typeof step.url === 'function' ? step.url(state.appState) : step.url;
  const resolvedHeaders = step.headers ? step.headers(state.appState) : {};
  const resolvedBody = step.body ? step.body(state.appState) : undefined;

  return (
    <div className={`${styles.card} ${styles[status.toLowerCase()]}`}>
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <h3 className={styles.title}>{step.title}</h3>
          <StatusBadge status={status} />
        </div>
        <p className={styles.description}>{step.description}</p>
      </div>
      <div className={styles.actions}>
        <button
          className={styles.runBtn}
          onClick={() => runStep(step)}
          disabled={isRunning || state.runAllInProgress}
        >
          Run
        </button>
      </div>
      <RequestPanel
        method={step.method}
        url={resolvedUrl}
        headers={resolvedHeaders}
        body={resolvedBody}
      />
      {result && result.status !== 'PENDING' && result.status !== 'RUNNING' && (
        <ResponsePanel result={result} />
      )}
    </div>
  );
}
