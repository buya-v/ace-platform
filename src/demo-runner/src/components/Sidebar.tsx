import React from 'react';
import { useDemo } from '../contexts/DemoContext';
import { allSections } from '../data/sections';
import { isChecklistSection } from '../types/section';
import styles from './Sidebar.module.css';

export function Sidebar() {
  const { state, dispatch } = useDemo();

  return (
    <nav className={styles.sidebar}>
      <div className={styles.logo}>ACE Demo Runner</div>
      <ul className={styles.list}>
        {allSections.map((section) => {
          const isActive = state.activeSectionId === section.id;
          const stepCount = isChecklistSection(section) ? section.items.length : section.steps.length;
          let doneCount = 0;
          if (!isChecklistSection(section)) {
            doneCount = section.steps.filter((s) => {
              const r = state.results[s.id];
              return r && r.status === 'PASS';
            }).length;
          } else {
            doneCount = section.items.filter((i) => state.checkedItems[i.id]).length;
          }

          return (
            <li key={section.id}>
              <button
                className={`${styles.item} ${isActive ? styles.active : ''}`}
                onClick={() => dispatch({ type: 'SET_ACTIVE_SECTION', sectionId: section.id })}
              >
                <span className={styles.title}>{section.title}</span>
                <span className={styles.count}>
                  {doneCount}/{stepCount}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
