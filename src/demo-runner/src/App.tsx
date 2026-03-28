import React from 'react';
import { useDemo } from './contexts/DemoContext';
import { allSections } from './data/sections';
import { isChecklistSection } from './types/section';
import { Sidebar } from './components/Sidebar';
import { TopBar } from './components/TopBar';
import { BottomBar } from './components/BottomBar';
import { StepCard } from './components/StepCard';
import { ReadinessChecklist } from './components/ReadinessChecklist';
import styles from './App.module.css';

export default function App() {
  const { state } = useDemo();
  const activeSection = allSections.find((s) => s.id === state.activeSectionId);

  return (
    <div className={styles.layout}>
      <Sidebar />
      <div className={styles.main}>
        <TopBar />
        <div className={styles.content}>
          {activeSection && (
            <>
              <h2 className={styles.sectionTitle}>{activeSection.title}</h2>
              {isChecklistSection(activeSection) ? (
                <ReadinessChecklist section={activeSection} />
              ) : (
                activeSection.steps.map((step) => <StepCard key={step.id} step={step} />)
              )}
            </>
          )}
        </div>
        <BottomBar />
      </div>
    </div>
  );
}
