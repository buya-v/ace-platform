import { StepDefinition, ChecklistItem } from './step';

export interface Section {
  id: string;
  title: string;
  steps: StepDefinition[];
}

export interface ChecklistSection {
  id: string;
  title: string;
  items: ChecklistItem[];
}

export type AnySection = Section | ChecklistSection;

export function isChecklistSection(s: AnySection): s is ChecklistSection {
  return 'items' in s;
}
