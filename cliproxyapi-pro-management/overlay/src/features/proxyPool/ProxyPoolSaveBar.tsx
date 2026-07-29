import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import styles from './ProxyPool.module.scss';

interface ProxyPoolSaveBarProps {
  visible: boolean;
  saving: boolean;
  onDiscard: () => void;
  onSave: () => void;
}

export function ProxyPoolSaveBar({ visible, saving, onDiscard, onSave }: ProxyPoolSaveBarProps) {
  const { t } = useTranslation();
  if (!visible) return null;
  const content = (
    <footer className={styles.saveBar}>
      <div>
        <strong>{t('proxy_pool.unsaved_changes', { defaultValue: 'Unsaved changes' })}</strong>
        <span>
          {t('proxy_pool.save_bar_hint', {
            defaultValue: 'Review and save to apply them to the running pool.',
          })}
        </span>
      </div>
      <div>
        <Button variant="ghost" size="sm" onClick={onDiscard} disabled={saving}>
          {t('proxy_pool.discard_changes', { defaultValue: 'Discard' })}
        </Button>
        <Button size="sm" onClick={onSave} loading={saving}>
          {t('common.save')}
        </Button>
      </div>
    </footer>
  );
  const target = typeof document !== 'undefined' ? document.querySelector('.main-body') : null;
  return target ? createPortal(content, target) : content;
}
