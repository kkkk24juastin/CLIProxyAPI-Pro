export interface ProfileUsageObservation {
  profileId: string;
  profileName: string;
  timestampMs: number;
}

export interface ProfileFilterOption {
  value: string;
  label: string;
}

export interface ProfileFilterCopy {
  allProfiles: string;
  renamed: (name: string, previous: string) => string;
  deleted: (name: string) => string;
}

interface ProfileNameHistory {
  latestName: string;
  latestTimestampMs: number;
  names: Map<string, number>;
}

export const resolveUsageProfileSnapshot = (
  profileName: string,
  profileId: string,
  emptyLabel: string,
): string => profileName.trim() || profileId.trim() || emptyLabel;

export const buildProfileFilterOptions = ({
  observations,
  currentNames,
  currentNamesLoaded,
  selectedProfileId,
  selectedProfileName,
  copy,
}: {
  observations: ProfileUsageObservation[];
  currentNames: ReadonlyMap<string, string>;
  currentNamesLoaded: boolean;
  selectedProfileId: string;
  selectedProfileName: string;
  copy: ProfileFilterCopy;
}): ProfileFilterOption[] => {
  const histories = new Map<string, ProfileNameHistory>();
  observations.forEach((observation) => {
    const profileId = observation.profileId.trim();
    const profileName = observation.profileName.trim();
    if (!profileId) return;
    const history = histories.get(profileId) ?? {
      latestName: '',
      latestTimestampMs: Number.NEGATIVE_INFINITY,
      names: new Map<string, number>(),
    };
    if (profileName) {
      const timestampMs = Number.isFinite(observation.timestampMs) ? observation.timestampMs : 0;
      history.names.set(profileName, Math.max(history.names.get(profileName) ?? Number.NEGATIVE_INFINITY, timestampMs));
      if (timestampMs >= history.latestTimestampMs) {
        history.latestName = profileName;
        history.latestTimestampMs = timestampMs;
      }
    }
    histories.set(profileId, history);
  });

  currentNames.forEach((_name, profileId) => {
    const normalizedId = profileId.trim();
    if (!normalizedId) return;
    const history = histories.get(normalizedId) ?? {
      latestName: '',
      latestTimestampMs: Number.NEGATIVE_INFINITY,
      names: new Map<string, number>(),
    };
    histories.set(normalizedId, history);
  });

  const normalizedSelectedId = selectedProfileId.trim();
  if (normalizedSelectedId && normalizedSelectedId !== 'all' && !histories.has(normalizedSelectedId)) {
    histories.set(normalizedSelectedId, {
      latestName: selectedProfileName.trim(),
      latestTimestampMs: Number.POSITIVE_INFINITY,
      names: new Map(selectedProfileName.trim() ? [[selectedProfileName.trim(), Number.POSITIVE_INFINITY]] : []),
    });
  }

  const options = Array.from(histories.entries()).map(([profileId, history]) => {
    const currentName = currentNames.get(profileId)?.trim() ?? '';
    const selectedName = profileId === normalizedSelectedId ? selectedProfileName.trim() : '';
    const primaryName = currentName || selectedName || history.latestName || profileId;
    const previousNames = Array.from(history.names.entries())
      .filter(([name]) => name !== primaryName)
      .sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0]))
      .map(([name]) => name);
    let label = previousNames.length > 0
      ? copy.renamed(primaryName, previousNames.join(', '))
      : primaryName;
    if (currentNamesLoaded && !currentNames.has(profileId)) {
      label = copy.deleted(label);
    }
    return { value: profileId, label };
  });

  return [
    { value: 'all', label: copy.allProfiles },
    ...options.sort((left, right) => left.label.localeCompare(right.label)),
  ];
};
