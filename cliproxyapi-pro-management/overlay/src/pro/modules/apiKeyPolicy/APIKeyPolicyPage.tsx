import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import {
  IconAlertTriangle,
	IconCheckCircle2,
	IconInfo,
  IconKey,
  IconPlus,
	IconShield,
  IconTrash2,
} from '@/components/ui/icons';
import { useAuthStore, useNotificationStore } from '@/stores';
import { ProFeatureHeader } from '@/pro/shared/ProFeatureHeader';
import { ProTaskDialog, ProWorkspaceSheet } from '@/pro/shared/ProSurface';
import {
  apiKeyPolicyApi,
  apiKeyPolicyErrorCode,
  apiKeyPolicyErrorTranslationKey,
  cloneProfileInput,
  isAPIKeyPolicyUnsupported,
  validateProfileInput,
  type APIKeyPolicy,
  type APIKeyPolicyBinding,
  type APIKeyPolicyCatalog,
  type APIKeyPolicyDeletePreview,
  type APIKeyPolicySnapshot,
  type APIKeyProfileInput,
} from './apiKeyPolicy';
import styles from './APIKeyPolicyPage.module.scss';

type BindingFilter = 'all' | 'unconfigured' | 'configured' | 'orphaned';
type CapabilityState = 'checking' | 'ready' | 'unsupported' | 'error';
type WorkspaceTarget =
  | { kind: 'create'; binding: APIKeyPolicyBinding }
  | { kind: 'policy'; policy: APIKeyPolicy; readOnly: boolean };

interface WorkspaceDraft {
  displayName: string;
  profileId: string;
  profile: APIKeyProfileInput;
  isNewProfile: boolean;
}

const emptyProfile = (catalog: APIKeyPolicyCatalog): APIKeyProfileInput => ({
  name: '',
  providers: catalog.providers.length === 1 ? [catalog.providers[0]] : [],
  models: [],
  mappings: [],
});

const workspaceDraftFromTarget = (
  target: WorkspaceTarget,
  catalog: APIKeyPolicyCatalog,
  profileId?: string,
): WorkspaceDraft => {
  if (target.kind === 'create') {
    return { displayName: '', profileId: '', profile: emptyProfile(catalog), isNewProfile: false };
  }
  const selected =
    target.policy.profiles.find((profile) => profile.id === profileId) ??
    target.policy.profiles.find((profile) => profile.id === target.policy.activeProfileId) ??
    target.policy.profiles[0];
  return {
    displayName: target.policy.displayName,
    profileId: selected?.id ?? '',
    profile: selected ? cloneProfileInput(selected) : emptyProfile(catalog),
    isNewProfile: false,
  };
};

const profileSignature = (profile: APIKeyProfileInput): string =>
  JSON.stringify({
    name: profile.name.trim(),
    providers: [...profile.providers].sort(),
    models: [...profile.models].sort(),
    mappings: profile.mappings.map(({ source, target }) => ({
      source: source.trim(),
      target: target.trim(),
    })),
  });

const workspaceIsDirty = (
  target: WorkspaceTarget | null,
  draft: WorkspaceDraft | null,
): boolean => {
  if (!target || !draft) return false;
  if (target.kind === 'create') {
    return Boolean(
      draft.displayName.trim() ||
        draft.profile.name.trim() ||
        draft.profile.providers.length ||
        draft.profile.models.length ||
        draft.profile.mappings.length,
    );
  }
  if (draft.isNewProfile) return true;
  const persisted = target.policy.profiles.find((profile) => profile.id === draft.profileId);
  return (
    draft.displayName !== target.policy.displayName ||
    !persisted ||
    profileSignature(draft.profile) !== profileSignature(persisted)
  );
};

const formatUpdatedAt = (value: number, language: string): string =>
  value > 0 ? new Intl.DateTimeFormat(language, { dateStyle: 'medium', timeStyle: 'short' }).format(value) : '-';

function ChoiceList({
  title,
  values,
  selected,
  onChange,
  disabled,
  emptyLabel,
}: {
  title: string;
  values: string[];
  selected: string[];
  onChange: (values: string[]) => void;
  disabled: boolean;
  emptyLabel: string;
}) {
  const [search, setSearch] = useState('');
  const query = search.trim().toLowerCase();
  const filtered = values.filter((value) => !query || value.toLowerCase().includes(query));
  return (
    <section className={styles.choiceSection}>
      <div className={styles.choiceHeader}>
        <strong>{title}</strong>
        <span>{selected.length}</span>
      </div>
      <input
        className="input"
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        placeholder={emptyLabel}
        disabled={disabled}
        aria-label={emptyLabel}
      />
      <div className={styles.choiceList}>
        {filtered.length ? (
          filtered.map((value) => (
            <label className={styles.choiceItem} key={value}>
              <input
                type="checkbox"
                checked={selected.includes(value)}
                disabled={disabled}
                onChange={(event) =>
                  onChange(
                    event.target.checked
                      ? [...selected, value]
                      : selected.filter((item) => item !== value),
                  )
                }
              />
              <span>{value}</span>
            </label>
          ))
        ) : (
          <span className={styles.choiceEmpty}>{emptyLabel}</span>
        )}
      </div>
    </section>
  );
}

function PolicyBadge({ state, children }: { state: string; children: ReactNode }) {
  return <span className={`${styles.badge} ${styles[`badge_${state}`] ?? ''}`}>{children}</span>;
}

export function APIKeyPolicyPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const connectionStatus = useAuthStore((state) => state.connectionStatus);
  const showNotification = useNotificationStore((state) => state.showNotification);
  const [snapshot, setSnapshot] = useState<APIKeyPolicySnapshot | null>(null);
  const [capability, setCapability] = useState<CapabilityState>('checking');
  const [loadError, setLoadError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const [filter, setFilter] = useState<BindingFilter>('all');
  const [search, setSearch] = useState('');
  const [workspaceTarget, setWorkspaceTarget] = useState<WorkspaceTarget | null>(null);
  const [draft, setDraft] = useState<WorkspaceDraft | null>(null);
  const [saving, setSaving] = useState(false);
	const [takeoverOpen, setTakeoverOpen] = useState(false);
	const [takeoverBusy, setTakeoverBusy] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [dangerPolicy, setDangerPolicy] = useState<APIKeyPolicy | null>(null);
  const [dangerKind, setDangerKind] = useState<'policy' | 'profile' | 'orphaned' | null>(null);
  const [dangerBusy, setDangerBusy] = useState(false);
  const [deletePreview, setDeletePreview] = useState<APIKeyPolicyDeletePreview | null>(null);
  const requestRevisionRef = useRef(0);
  const saveRevisionRef = useRef(0);
  const draftRevisionRef = useRef(0);
  const savingRef = useRef(false);
  const dangerRevisionRef = useRef(0);
  const dangerBusyRef = useRef(false);
  const dirty = workspaceIsDirty(workspaceTarget, draft);

  const errorMessage = useCallback((error: unknown): string => {
    const key = apiKeyPolicyErrorTranslationKey(error);
    if (key && i18n.exists(key)) return t(key);
    return error instanceof Error ? error.message : String(error ?? '');
  }, [i18n, t]);

  const openPolicyDeletePreview = useCallback(async (policy: APIKeyPolicy) => {
    if (dangerBusyRef.current) return;
    const revision = ++dangerRevisionRef.current;
    dangerBusyRef.current = true;
    setDangerBusy(true);
    setDeletePreview(null);
    try {
      const preview = await apiKeyPolicyApi.deletePreview(policy.id);
      if (revision !== dangerRevisionRef.current) return;
      if (preview.policyId !== policy.id || preview.version !== policy.version) {
        throw new Error(t('api_key_policy.delete_preview_stale'));
      }
      setDeletePreview(preview);
      setDangerPolicy(policy);
      setDangerKind('policy');
    } catch (error) {
      if (revision !== dangerRevisionRef.current) return;
      showNotification(errorMessage(error), 'error');
    } finally {
      if (revision === dangerRevisionRef.current) {
        dangerBusyRef.current = false;
        setDangerBusy(false);
      }
    }
  }, [errorMessage, showNotification, t]);

  const load = useCallback(async () => {
    if (connectionStatus !== 'connected') {
      setLoading(false);
      setCapability('checking');
      return;
    }
    const revision = ++requestRevisionRef.current;
    setLoading(true);
    setLoadError(null);
    try {
      const next = await apiKeyPolicyApi.snapshot();
      if (revision !== requestRevisionRef.current) return;
      setSnapshot(next);
      setCapability('ready');
    } catch (error) {
      if (revision !== requestRevisionRef.current) return;
      if (isAPIKeyPolicyUnsupported(error)) {
        setCapability('unsupported');
        setSnapshot(null);
      } else {
        setCapability('error');
        setLoadError(error);
      }
    } finally {
      if (revision === requestRevisionRef.current) setLoading(false);
    }
  }, [connectionStatus]);

  useEffect(() => {
    void load();
    return () => {
      requestRevisionRef.current += 1;
      saveRevisionRef.current += 1;
      dangerRevisionRef.current += 1;
      savingRef.current = false;
      dangerBusyRef.current = false;
    };
  }, [load]);

  const openWorkspace = useCallback(
    (target: WorkspaceTarget, profileId?: string) => {
      if (!snapshot) return;
      setWorkspaceTarget(target);
      draftRevisionRef.current += 1;
      setDraft(workspaceDraftFromTarget(target, snapshot.catalog, profileId));
      setConflict(false);
    },
    [snapshot],
  );

  const closeWorkspace = useCallback(() => {
    saveRevisionRef.current += 1;
    draftRevisionRef.current += 1;
    savingRef.current = false;
    dangerRevisionRef.current += 1;
    dangerBusyRef.current = false;
    setSaving(false);
    setDangerBusy(false);
    setDeletePreview(null);
    setWorkspaceTarget(null);
    setDraft(null);
    setConflict(false);
  }, []);

	const requestWorkspaceClose = useCallback(async () => {
		if (dirty && !window.confirm(t('api_key_policy.discard_confirm'))) return;
		closeWorkspace();
	}, [closeWorkspace, dirty, t]);

	const toggleTakeover = useCallback(async () => {
		if (!snapshot || takeoverBusy) return;
		const enabled = !snapshot.takeoverEnabled;
		setTakeoverBusy(true);
		try {
			const status = await apiKeyPolicyApi.setTakeover(enabled);
			setSnapshot((current) => current
				? { ...current, takeoverEnabled: status.takeoverEnabled }
				: current);
			setTakeoverOpen(false);
			showNotification(
				status.takeoverEnabled
					? t('api_key_policy.takeover_started')
					: t('api_key_policy.takeover_stopped'),
				'success',
			);
		} catch (error) {
			showNotification(errorMessage(error), 'error');
		} finally {
			setTakeoverBusy(false);
		}
	}, [errorMessage, showNotification, snapshot, t, takeoverBusy]);

  const updateDraft = useCallback((updater: (current: WorkspaceDraft) => WorkspaceDraft) => {
    draftRevisionRef.current += 1;
    setDraft((current) => current ? updater(current) : current);
    setConflict(false);
  }, []);

  const replacePolicyInSnapshot = useCallback((policy: APIKeyPolicy) => {
    setSnapshot((current) => {
      if (!current) return current;
      return {
        ...current,
        bindings: {
          ...current.bindings,
          items: current.bindings.items.map((binding) =>
            binding.policy?.id === policy.id
              ? { ...binding, state: policy.state, policy }
              : binding,
          ),
          orphaned: current.bindings.orphaned.map((item) => (item.id === policy.id ? policy : item)),
        },
      };
    });
  }, []);

  const reloadWorkspace = useCallback(async () => {
    if (!workspaceTarget || workspaceTarget.kind !== 'policy' || !snapshot) return;
    const revision = ++saveRevisionRef.current;
    savingRef.current = true;
    setSaving(true);
    try {
      const policy = await apiKeyPolicyApi.get(workspaceTarget.policy.id);
      if (revision !== saveRevisionRef.current) return;
      const target = { kind: 'policy' as const, policy, readOnly: policy.state === 'orphaned' };
      setWorkspaceTarget(target);
      replacePolicyInSnapshot(policy);
      setConflict(false);
    } catch (error) {
      showNotification(errorMessage(error), 'error');
    } finally {
      if (revision === saveRevisionRef.current) {
        savingRef.current = false;
        setSaving(false);
      }
    }
  }, [errorMessage, replacePolicyInSnapshot, showNotification, snapshot, workspaceTarget]);

  const validateDraft = useCallback((): boolean => {
    if (!snapshot || !draft) return false;
    if (!draft.displayName.trim()) {
      showNotification(t('api_key_policy.validation.display_name'), 'warning');
      return false;
    }
    const problem = validateProfileInput(draft.profile, snapshot.catalog);
    if (problem) {
      showNotification(t(`api_key_policy.validation.${problem}`), 'warning');
      return false;
    }
    return true;
  }, [draft, showNotification, snapshot, t]);

  const saveWorkspace = useCallback(async () => {
    if (!workspaceTarget || !draft || !validateDraft() || savingRef.current) return;
    const revision = ++saveRevisionRef.current;
    const submittedDraftRevision = draftRevisionRef.current;
    savingRef.current = true;
    setSaving(true);
    setConflict(false);
    try {
      let policy: APIKeyPolicy;
      if (workspaceTarget.kind === 'create') {
        policy = await apiKeyPolicyApi.create(
          workspaceTarget.binding.keyRef,
          draft.displayName.trim(),
          draft.profile,
        );
      } else {
        const persisted = workspaceTarget.policy.profiles.find((item) => item.id === draft.profileId);
        const changedProfile = draft.isNewProfile || !persisted ||
          profileSignature(persisted) !== profileSignature(draft.profile);
        policy = await apiKeyPolicyApi.updateWorkspace(
          workspaceTarget.policy.id,
          draft.displayName.trim(),
          workspaceTarget.policy.version,
          draft.profileId,
          changedProfile ? draft.profile : undefined,
          draft.isNewProfile,
        );
      }
      if (revision !== saveRevisionRef.current) return;
      replacePolicyInSnapshot(policy);
      const target = { kind: 'policy' as const, policy, readOnly: false };
      setWorkspaceTarget(target);
      const priorProfileIDs = workspaceTarget.kind === 'policy'
        ? new Set(workspaceTarget.policy.profiles.map((item) => item.id))
        : new Set<string>();
      const savedProfile = draft.isNewProfile
        ? policy.profiles.find((item) => !priorProfileIDs.has(item.id))
        : policy.profiles.find((item) => item.id === draft.profileId);
      if (submittedDraftRevision === draftRevisionRef.current) {
        draftRevisionRef.current += 1;
        setDraft(workspaceDraftFromTarget(target, snapshot!.catalog, savedProfile?.id));
      }
      showNotification(t('api_key_policy.saved'), 'success');
      await load();
    } catch (error) {
      if (revision !== saveRevisionRef.current) return;
      if (apiKeyPolicyErrorCode(error) === 'config_version_conflict') {
        setConflict(true);
      } else if (apiKeyPolicyErrorCode(error) === 'api_key_reference_stale') {
        showNotification(t('api_key_policy.key_ref_stale'), 'warning');
        closeWorkspace();
        await load();
      } else {
        showNotification(errorMessage(error), 'error');
      }
    } finally {
      if (revision === saveRevisionRef.current) {
        savingRef.current = false;
        setSaving(false);
      }
    }
  }, [closeWorkspace, draft, errorMessage, load, replacePolicyInSnapshot, showNotification, snapshot, t, validateDraft, workspaceTarget]);

  const activateProfile = useCallback(async () => {
    if (!workspaceTarget || workspaceTarget.kind !== 'policy' || !draft || dirty || savingRef.current) return;
    const revision = ++saveRevisionRef.current;
    const submittedDraftRevision = draftRevisionRef.current;
    savingRef.current = true;
    setSaving(true);
    try {
      const policy = await apiKeyPolicyApi.activate(
        workspaceTarget.policy.id,
        draft.profileId,
        workspaceTarget.policy.version,
      );
      if (revision !== saveRevisionRef.current) return;
      replacePolicyInSnapshot(policy);
      const target = { kind: 'policy' as const, policy, readOnly: false };
      setWorkspaceTarget(target);
      if (submittedDraftRevision === draftRevisionRef.current) {
        draftRevisionRef.current += 1;
        setDraft(workspaceDraftFromTarget(target, snapshot!.catalog, draft.profileId));
      }
      showNotification(t('api_key_policy.activated'), 'success');
    } catch (error) {
      if (revision !== saveRevisionRef.current) return;
      if (apiKeyPolicyErrorCode(error) === 'config_version_conflict') setConflict(true);
      else showNotification(errorMessage(error), 'error');
    } finally {
      if (revision === saveRevisionRef.current) {
        savingRef.current = false;
        setSaving(false);
      }
    }
  }, [dirty, draft, errorMessage, replacePolicyInSnapshot, showNotification, snapshot, t, workspaceTarget]);

  const runDangerAction = useCallback(async () => {
    if (!dangerPolicy || !dangerKind || dangerBusyRef.current) return;
    if (dangerKind === 'policy' && (
      !deletePreview || deletePreview.policyId !== dangerPolicy.id || deletePreview.version !== dangerPolicy.version
    )) return;
    const revision = ++dangerRevisionRef.current;
    dangerBusyRef.current = true;
    setDangerBusy(true);
    try {
      if (dangerKind === 'policy') {
        await apiKeyPolicyApi.deletePolicy(dangerPolicy.id, dangerPolicy.version);
      } else if (dangerKind === 'orphaned') {
        if (!snapshot) return;
        await apiKeyPolicyApi.purgeOrphaned(
          dangerPolicy.id,
          dangerPolicy.version,
          snapshot.bindings.configGeneration,
        );
      } else if (workspaceTarget?.kind === 'policy' && draft) {
        await apiKeyPolicyApi.deleteProfile(dangerPolicy.id, draft.profileId, dangerPolicy.version);
      }
      if (revision !== dangerRevisionRef.current) return;
      setDangerPolicy(null);
      setDangerKind(null);
      setDeletePreview(null);
      closeWorkspace();
      await load();
      showNotification(t('api_key_policy.deleted'), 'success');
    } catch (error) {
      if (revision !== dangerRevisionRef.current) return;
      if (apiKeyPolicyErrorCode(error) === 'config_version_conflict') {
        setConflict(true);
        setDangerPolicy(null);
        setDangerKind(null);
        setDeletePreview(null);
      }
      showNotification(errorMessage(error), 'error');
    } finally {
      if (revision === dangerRevisionRef.current) {
        dangerBusyRef.current = false;
        setDangerBusy(false);
      }
    }
  }, [closeWorkspace, dangerKind, dangerPolicy, deletePreview, draft, errorMessage, load, showNotification, snapshot, t, workspaceTarget]);

  const visibleItems = useMemo(() => {
    const query = search.trim().toLowerCase();
    const current = snapshot?.bindings.items ?? [];
    const associated = current.filter((binding) => {
      if (filter !== 'all' && filter !== binding.state) return false;
      const text = `${binding.maskedKey} ${binding.policy?.displayName ?? ''} ${binding.policy?.profiles.map((profile) => profile.name).join(' ') ?? ''}`.toLowerCase();
      return !query || text.includes(query);
    });
    const orphaned = (snapshot?.bindings.orphaned ?? []).filter((policy) => {
      if (filter !== 'all' && filter !== 'orphaned') return false;
      const text = `${policy.displayName} ${policy.profiles.map((profile) => profile.name).join(' ')}`.toLowerCase();
      return !query || text.includes(query);
    });
    return { associated, orphaned };
  }, [filter, search, snapshot]);

  const statusCounts = useMemo(() => ({
    all: (snapshot?.bindings.items.length ?? 0) + (snapshot?.bindings.orphaned.length ?? 0),
    unconfigured: snapshot?.bindings.items.filter((item) => item.state === 'unconfigured').length ?? 0,
    configured: snapshot?.bindings.items.filter((item) => item.state === 'configured').length ?? 0,
    orphaned: snapshot?.bindings.orphaned.length ?? 0,
  }), [snapshot]);

  const currentPolicy = workspaceTarget?.kind === 'policy' ? workspaceTarget.policy : null;
  const readOnly = workspaceTarget?.kind === 'policy' && workspaceTarget.readOnly;
  const selectedProfile = currentPolicy?.profiles.find((profile) => profile.id === draft?.profileId);
  const active = Boolean(currentPolicy && draft?.profileId === currentPolicy.activeProfileId);

  useEffect(() => {
    if (!snapshot || workspaceTarget || capability !== 'ready') return;
    const params = new URLSearchParams(location.search);
    const policyId = params.get('policy_id')?.trim();
    const profileId = params.get('profile_id')?.trim();
    if (!policyId) return;
    const policy = [
      ...snapshot.bindings.items.flatMap((item) => (item.policy ? [item.policy] : [])),
      ...snapshot.bindings.orphaned,
    ].find((item) => item.id === policyId);
    if (policy) openWorkspace({ kind: 'policy', policy, readOnly: policy.state === 'orphaned' }, profileId);
  }, [capability, location.search, openWorkspace, snapshot, workspaceTarget]);

  return (
    <div className={styles.page}>
		<ProFeatureHeader
			title={t('api_key_policy.title')}
			subtitle={t('api_key_policy.subtitle')}
			icon={<IconKey size={20} />}
			active={snapshot?.takeoverEnabled === true}
			loading={loading}
			actionBusy={takeoverBusy}
			actionDisabled={!snapshot || capability !== 'ready'}
			onRefresh={() => void load()}
			onToggle={() => setTakeoverOpen(true)}
		/>

      {capability === 'unsupported' ? (
        <section className={styles.capabilityNotice}>
          <IconAlertTriangle size={22} />
          <div><strong>{t('api_key_policy.unsupported_title')}</strong><p>{t('api_key_policy.unsupported_body')}</p></div>
        </section>
      ) : capability === 'error' ? (
        <section className={styles.capabilityNotice}>
          <IconAlertTriangle size={22} />
          <div><strong>{t('api_key_policy.unavailable_title')}</strong><p>{errorMessage(loadError)}</p></div>
        </section>
      ) : (
        <>
			<section className={styles.statusOverview} aria-label={t('api_key_policy.overview')}>
				<div className={styles.overviewItem}>
					<span className={snapshot?.takeoverEnabled ? styles.overviewGood : styles.overviewMuted}>
						{snapshot?.takeoverEnabled ? <IconCheckCircle2 size={18} /> : <IconAlertTriangle size={18} />}
					</span>
					<div><small>{t('api_key_policy.runtime')}</small><strong>{snapshot?.takeoverEnabled ? t('api_key_policy.running') : t('api_key_policy.stopped')}</strong></div>
				</div>
				<div className={styles.overviewItem}>
					<span className={styles.overviewAccent}><IconKey size={18} /></span>
					<div><small>{t('api_key_policy.upstream_keys')}</small><strong>{snapshot?.bindings.items.length ?? 0}</strong></div>
				</div>
				<div className={styles.overviewItem}>
					<span className={styles.overviewGood}><IconShield size={18} /></span>
					<div><small>{t('api_key_policy.configured_policies')}</small><strong>{statusCounts.configured}</strong></div>
				</div>
				<div className={styles.overviewItem}>
					<span className={statusCounts.orphaned ? styles.overviewBad : styles.overviewMuted}><IconAlertTriangle size={18} /></span>
					<div><small>{t('api_key_policy.orphaned_policies')}</small><strong>{statusCounts.orphaned}</strong></div>
				</div>
			</section>

          <div className={styles.toolbar}>
            <input className="input" value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t('api_key_policy.search')} />
            <Select
              value={filter}
              onChange={(value) => setFilter(value as BindingFilter)}
              options={(['all', 'unconfigured', 'configured', 'orphaned'] as const).map((value) => ({ value, label: t(`api_key_policy.filter.${value}`) }))}
              ariaLabel={t('api_key_policy.filter_label')}
            />
				<Button variant="secondary" size="sm" onClick={() => navigate('/config?section=api-keys')}>
					{t('api_key_policy.manage_upstream_keys')}
				</Button>
          </div>

          {loading && !snapshot ? <div className={styles.empty}>{t('common.loading')}</div> : null}

          <section className={styles.cards} aria-busy={loading}>
            {visibleItems.associated.map((binding) => {
              const policy = binding.policy;
              const activeProfile = policy?.profiles.find((profile) => profile.id === policy.activeProfileId);
              return (
                <article className={styles.card} key={`${binding.maskedKey}-${policy?.id ?? 'unconfigured'}`}>
                  <div className={styles.cardTop}>
                    <div className={styles.cardIdentity}>
                      <span><IconKey size={18} /></span>
                      <div><strong>{policy?.displayName || binding.maskedKey}</strong><code>{binding.maskedKey}</code></div>
                    </div>
                    <PolicyBadge state={binding.state}>{t(`api_key_policy.state.${binding.state}`)}</PolicyBadge>
                  </div>
                  <p className={styles.cardSummary}>
                    {policy
							? t(snapshot?.takeoverEnabled ? 'api_key_policy.configured_summary' : 'api_key_policy.configured_inactive_summary', { profile: activeProfile?.name ?? '-', count: policy.profiles.length })
                      : t('api_key_policy.passthrough_summary')}
                  </p>
                  {binding.weakKey ? <div className={styles.weakKey}><IconAlertTriangle size={15} /> {t('api_key_policy.weak_key')}</div> : null}
                  <div className={styles.cardMeta}>
                    <span>{t('api_key_policy.active_profile')}: <strong>{activeProfile?.name ?? '-'}</strong></span>
                    <span>{t('api_key_policy.updated')}: {policy ? formatUpdatedAt(policy.updatedAtMs, i18n.resolvedLanguage ?? i18n.language) : '-'}</span>
                  </div>
                  <div className={styles.cardActions}>
                    {policy ? (
                      <>
                        <Button variant="secondary" size="sm" onClick={() => openWorkspace({ kind: 'policy', policy, readOnly: false })}>{t('api_key_policy.open_workspace')}</Button>
                        <Button variant="ghost" size="sm" onClick={() => navigate(`/monitoring?api_key_policy_id=${encodeURIComponent(policy.id)}`)}>{t('api_key_policy.view_usage')}</Button>
                      </>
                    ) : (
                      <Button size="sm" onClick={() => openWorkspace({ kind: 'create', binding })}>{t('api_key_policy.configure')}</Button>
                    )}
                  </div>
                </article>
              );
            })}
          </section>

          {visibleItems.orphaned.length ? (
            <section className={styles.orphanedSection}>
              <div className={styles.sectionHeading}><div><h2>{t('api_key_policy.orphaned_title')}</h2><p>{t('api_key_policy.orphaned_body')}</p></div><PolicyBadge state="orphaned">{visibleItems.orphaned.length}</PolicyBadge></div>
              <div className={styles.cards}>
                {visibleItems.orphaned.map((policy) => (
                  <article className={`${styles.card} ${styles.orphanedCard}`} key={policy.id}>
                    <div className={styles.cardTop}><div className={styles.cardIdentity}><span><IconAlertTriangle size={18} /></span><div><strong>{policy.displayName}</strong><code>{policy.id}</code></div></div><PolicyBadge state="orphaned">{t('api_key_policy.state.orphaned')}</PolicyBadge></div>
                    <p className={styles.cardSummary}>{t('api_key_policy.orphaned_summary', { count: policy.profiles.length })}</p>
                    <div className={styles.cardActions}>
                      <Button variant="secondary" size="sm" onClick={() => openWorkspace({ kind: 'policy', policy, readOnly: true })}>{t('api_key_policy.inspect')}</Button>
                      <Button variant="danger" size="sm" onClick={() => { setDangerPolicy(policy); setDangerKind('orphaned'); }}>{t('api_key_policy.purge')}</Button>
                    </div>
                  </article>
                ))}
              </div>
            </section>
          ) : null}

          {!loading && snapshot && visibleItems.associated.length === 0 && visibleItems.orphaned.length === 0 ? <div className={styles.empty}>{t('api_key_policy.empty')}</div> : null}
        </>
      )}

      <ProWorkspaceSheet
        open={Boolean(workspaceTarget && draft)}
		onClose={closeWorkspace}
        confirmClose={() => !dirty || window.confirm(t('api_key_policy.discard_confirm'))}
        closeDisabled={saving}
		size="lg"
		className={styles.policySheet}
        eyebrow={workspaceTarget?.kind === 'create' ? t('api_key_policy.create_eyebrow') : t('api_key_policy.workspace_eyebrow')}
        title={workspaceTarget?.kind === 'create' ? t('api_key_policy.create_title') : currentPolicy?.displayName}
        description={readOnly ? t('api_key_policy.orphaned_read_only') : t('api_key_policy.workspace_description')}
		footer={
			<div className={styles.sheetFooter}>
				<span>{dirty ? t('config_management.status_dirty_short') : t('api_key_policy.workspace_saved')}</span>
				<div>
					<Button variant="secondary" onClick={() => void requestWorkspaceClose()} disabled={saving}>
						{readOnly ? t('common.close') : t('common.cancel')}
					</Button>
					{!readOnly ? <Button onClick={() => void saveWorkspace()} loading={saving} disabled={!dirty || saving}>{t('common.save')}</Button> : null}
				</div>
			</div>
		}
      >
        {draft && snapshot ? (
          <div className={styles.workspace}>
            {conflict ? (
              <div className={styles.conflict} role="alert">
                <div><strong>{t('api_key_policy.conflict_title')}</strong><p>{t('api_key_policy.conflict_body')}</p></div>
                <Button variant="secondary" size="sm" onClick={() => void reloadWorkspace()} disabled={saving}>{t('api_key_policy.reload_server')}</Button>
              </div>
            ) : null}

            <div className={styles.workspaceTop}>
              <Input
                label={t('api_key_policy.display_name')}
                value={draft.displayName}
                onChange={(event) => updateDraft((current) => ({ ...current, displayName: event.target.value }))}
                disabled={readOnly || saving}
                hint={t('api_key_policy.display_name_hint')}
              />
              {currentPolicy ? <div className={styles.versionBox}><span>{t('api_key_policy.version')}</span><strong>{currentPolicy.version}</strong></div> : null}
            </div>

            {currentPolicy ? (
              <div className={styles.profileRail}>
                {currentPolicy.profiles.map((profile) => (
                  <button
                    key={profile.id}
                    className={draft.profileId === profile.id && !draft.isNewProfile ? styles.profileActive : ''}
                    onClick={() => updateDraft(() => workspaceDraftFromTarget(workspaceTarget!, snapshot.catalog, profile.id))}
                    disabled={saving || dirty}
                  >
                    <span>{profile.name}</span>
                    {profile.id === currentPolicy.activeProfileId ? <small>{t('api_key_policy.active')}</small> : null}
                  </button>
                ))}
                {!readOnly ? <button className={draft.isNewProfile ? styles.profileActive : ''} onClick={() => updateDraft((current) => ({ displayName: current.displayName, profileId: '', profile: emptyProfile(snapshot.catalog), isNewProfile: true }))} disabled={saving || dirty}><IconPlus size={14} /> {t('api_key_policy.new_profile')}</button> : null}
              </div>
            ) : null}

            <div className={styles.profileHeading}>
              <div><h3>{draft.isNewProfile ? t('api_key_policy.new_profile') : draft.profile.name || t('api_key_policy.initial_profile')}</h3><p>{active ? t('api_key_policy.active_profile_hint') : t('api_key_policy.inactive_profile_hint')}</p></div>
              {currentPolicy && !readOnly ? (
                <div className={styles.profileActions}>
                  {!active && !draft.isNewProfile ? <Button size="sm" onClick={() => void activateProfile()} disabled={dirty || saving}>{t('api_key_policy.set_active')}</Button> : null}
                  {!active && !draft.isNewProfile ? <Button variant="danger" size="sm" onClick={() => { setDangerPolicy(currentPolicy); setDangerKind('profile'); }} disabled={saving}><IconTrash2 size={14} /> {t('common.delete')}</Button> : null}
                </div>
              ) : null}
            </div>

            <Input
              label={t('api_key_policy.profile_name')}
              value={draft.profile.name}
              onChange={(event) => updateDraft((current) => ({ ...current, profile: { ...current.profile, name: event.target.value } }))}
              disabled={readOnly || saving}
            />

            <div className={styles.policyGrid}>
              <ChoiceList title={t('api_key_policy.allowed_providers')} values={snapshot.catalog.providers} selected={draft.profile.providers} onChange={(providers) => updateDraft((current) => ({ ...current, profile: { ...current.profile, providers } }))} disabled={Boolean(readOnly || saving)} emptyLabel={t('api_key_policy.search_providers')} />
              <ChoiceList title={t('api_key_policy.allowed_models')} values={snapshot.catalog.models} selected={draft.profile.models} onChange={(models) => updateDraft((current) => ({ ...current, profile: { ...current.profile, models, mappings: current.profile.mappings.filter((mapping) => models.includes(mapping.target)) } }))} disabled={Boolean(readOnly || saving)} emptyLabel={t('api_key_policy.search_models')} />
            </div>

            <section className={styles.mappingSection}>
              <div className={styles.mappingHeader}><div><h3>{t('api_key_policy.mappings')}</h3><p>{t('api_key_policy.mappings_hint')}</p></div>{!readOnly ? <Button variant="secondary" size="sm" onClick={() => updateDraft((current) => ({ ...current, profile: { ...current.profile, mappings: [...current.profile.mappings, { source: '', target: current.profile.models[0] ?? '' }] } }))} disabled={saving || draft.profile.models.length === 0}><IconPlus size={14} /> {t('common.add')}</Button> : null}</div>
              {draft.profile.mappings.length ? draft.profile.mappings.map((mapping, index) => (
                <div className={styles.mappingRow} key={`${index}-${mapping.source}`}>
                  <input className="input" value={mapping.source} placeholder={t('api_key_policy.source_model')} disabled={readOnly || saving} onChange={(event) => updateDraft((current) => { const mappings = current.profile.mappings.map((item, itemIndex) => itemIndex === index ? { ...item, source: event.target.value } : item); return { ...current, profile: { ...current.profile, mappings } }; })} />
                  <span>→</span>
                  <Select value={mapping.target} options={draft.profile.models.map((model) => ({ value: model, label: model }))} onChange={(target) => updateDraft((current) => { const mappings = current.profile.mappings.map((item, itemIndex) => itemIndex === index ? { ...item, target } : item); return { ...current, profile: { ...current.profile, mappings } }; })} disabled={Boolean(readOnly || saving)} ariaLabel={t('api_key_policy.target_model')} />
                  {!readOnly ? <button className={styles.iconButton} onClick={() => updateDraft((current) => ({ ...current, profile: { ...current.profile, mappings: current.profile.mappings.filter((_, itemIndex) => itemIndex !== index) } }))} disabled={saving} aria-label={t('common.delete')}><IconTrash2 size={15} /></button> : null}
                </div>
              )) : <div className={styles.mappingEmpty}>{t('api_key_policy.no_mappings')}</div>}
            </section>

            {currentPolicy ? (
              <div className={styles.workspaceFooterActions}>
                <Button variant="ghost" size="sm" onClick={() => navigate(`/monitoring?api_key_policy_id=${encodeURIComponent(currentPolicy.id)}${draft.profileId ? `&profile_id=${encodeURIComponent(draft.profileId)}` : ''}`)}>{t('api_key_policy.view_profile_usage')}</Button>
                {!readOnly ? <Button variant="danger" size="sm" onClick={() => void openPolicyDeletePreview(currentPolicy)} disabled={saving || dangerBusy}><IconAlertTriangle size={14} /> {t('api_key_policy.delete_policy')}</Button> : null}
              </div>
            ) : null}
          </div>
        ) : null}
      </ProWorkspaceSheet>

		<ProTaskDialog
			open={takeoverOpen}
			onClose={() => setTakeoverOpen(false)}
			closeDisabled={takeoverBusy}
			title={snapshot?.takeoverEnabled ? t('api_key_policy.stop_takeover_title') : t('api_key_policy.start_takeover_title')}
			footer={<><Button variant="secondary" onClick={() => setTakeoverOpen(false)} disabled={takeoverBusy}>{t('common.cancel')}</Button><Button variant={snapshot?.takeoverEnabled ? 'danger' : 'primary'} onClick={() => void toggleTakeover()} loading={takeoverBusy}>{snapshot?.takeoverEnabled ? t('pro_feature_header.stop_takeover') : t('pro_feature_header.start_takeover')}</Button></>}
		>
			<div className={styles.takeoverBody}>
				<span><IconInfo size={22} /></span>
				<div>
					<p>{snapshot?.takeoverEnabled ? t('api_key_policy.stop_takeover_body') : t('api_key_policy.start_takeover_body')}</p>
					<ul>
						<li>{t('api_key_policy.takeover_configured_count', { count: statusCounts.configured })}</li>
						<li>{t('api_key_policy.takeover_passthrough_count', { count: statusCounts.unconfigured })}</li>
						<li>{t('api_key_policy.takeover_new_requests_only')}</li>
					</ul>
				</div>
			</div>
		</ProTaskDialog>

      <ProTaskDialog
        open={Boolean(dangerPolicy && dangerKind)}
        onClose={() => { if (!dangerBusy) { setDangerPolicy(null); setDangerKind(null); setDeletePreview(null); } }}
        closeDisabled={dangerBusy}
        title={dangerKind === 'policy' ? t('api_key_policy.delete_policy_title') : dangerKind === 'orphaned' ? t('api_key_policy.purge_title') : t('api_key_policy.delete_profile_title')}
        footer={<><Button variant="secondary" onClick={() => { setDangerPolicy(null); setDangerKind(null); setDeletePreview(null); }} disabled={dangerBusy}>{t('common.cancel')}</Button><Button variant="danger" onClick={() => void runDangerAction()} loading={dangerBusy} disabled={dangerKind === 'policy' && !deletePreview}>{dangerKind === 'policy' ? t('api_key_policy.confirm_passthrough') : t('common.delete')}</Button></>}
      >
        <div className={styles.dangerBody}>
          <span><IconAlertTriangle size={24} /></span>
          <div>
            <strong>{dangerPolicy?.displayName}</strong>
            <p>{dangerKind === 'policy' ? t('api_key_policy.delete_policy_preview') : dangerKind === 'orphaned' ? t('api_key_policy.purge_preview') : t('api_key_policy.delete_profile_preview', { profile: selectedProfile?.name ?? '' })}</p>
            {dangerKind === 'policy' && deletePreview ? <ul><li>{t('api_key_policy.permission_change_models_count', { count: deletePreview.activeProfile.models.length })}</li><li>{t('api_key_policy.permission_change_providers_count', { count: deletePreview.activeProfile.providers.length })}</li><li>{t('api_key_policy.permission_change_new_requests')}</li></ul> : null}
          </div>
        </div>
      </ProTaskDialog>

    </div>
  );
}
