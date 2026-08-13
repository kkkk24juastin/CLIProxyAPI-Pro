import type { ReactNode } from 'react';
import styles from './ProFeatureTabs.module.scss';

export interface ProFeatureTabItem {
  key: string;
  label: ReactNode;
  icon?: ReactNode;
  badge?: ReactNode;
  disabled?: boolean;
}

interface ProFeatureTabsProps {
  items: ProFeatureTabItem[];
  activeKey: string;
  ariaLabel: string;
  onChange: (key: string) => void;
  className?: string;
}

export function ProFeatureTabs({
  items,
  activeKey,
  ariaLabel,
  onChange,
  className = '',
}: ProFeatureTabsProps) {
  return (
    <div
      className={`${styles.tabs} ${className}`.trim()}
      role="tablist"
      aria-label={ariaLabel}
      data-pro-feature-tabs
    >
      {items.map((item) => {
        const active = item.key === activeKey;
        return (
          <button
            key={item.key}
            type="button"
            role="tab"
            aria-selected={active}
            className={`${styles.tab} ${active ? styles.tabActive : ''}`}
            disabled={item.disabled}
            onClick={() => onChange(item.key)}
          >
            {item.icon ? <span className={styles.icon}>{item.icon}</span> : null}
            <span className={styles.label}>{item.label}</span>
            {item.badge !== undefined && item.badge !== null ? (
              <small className={styles.badge}>{item.badge}</small>
            ) : null}
          </button>
        );
      })}
    </div>
  );
}
