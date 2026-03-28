import React from 'react';
import { ChecklistSection } from '../types/section';
import { useDemo } from '../contexts/DemoContext';
import styles from './ReadinessChecklist.module.css';

interface ReadinessChecklistProps {
  section: ChecklistSection;
}

export function ReadinessChecklist({ section }: ReadinessChecklistProps) {
  const { state, dispatch } = useDemo();
  const categories = [...new Set(section.items.map((i) => i.category))];

  const checkedCount = section.items.filter((i) => state.checkedItems[i.id]).length;
  const total = section.items.length;
  const pct = total > 0 ? Math.round((checkedCount / total) * 100) : 0;

  return (
    <div className={styles.container}>
      <div className={styles.summary}>
        <span className={styles.pct}>{pct}%</span> Production Ready ({checkedCount}/{total})
      </div>
      {categories.map((cat) => (
        <div key={cat} className={styles.category}>
          <h4 className={styles.catTitle}>{cat}</h4>
          {section.items
            .filter((i) => i.category === cat)
            .map((item) => (
              <label key={item.id} className={styles.item}>
                <input
                  type="checkbox"
                  checked={!!state.checkedItems[item.id]}
                  onChange={() => dispatch({ type: 'TOGGLE_CHECKLIST_ITEM', itemId: item.id })}
                />
                <span className={styles.desc}>{item.description}</span>
                <span className={`${styles.status} ${styles[item.status.replace(/\s/g, '').toLowerCase()]}`}>
                  {item.status}
                </span>
              </label>
            ))}
        </div>
      ))}
    </div>
  );
}

export function getReadinessPercentage(
  items: { id: string }[],
  checkedItems: Record<string, boolean>,
): number {
  const total = items.length;
  if (total === 0) return 0;
  const checked = items.filter((i) => checkedItems[i.id]).length;
  return Math.round((checked / total) * 100);
}
