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
  deleted: (name: string) => string;
}

interface ProfileNameObservation {
  latestName: string;
  latestTimestampMs: number;
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
  const observedNames = new Map<string, ProfileNameObservation>();
  observations.forEach((observation) => {
    const profileId = observation.profileId.trim();
    const profileName = observation.profileName.trim();
    if (!profileId) return;
    const observed = observedNames.get(profileId) ?? {
      latestName: '',
      latestTimestampMs: Number.NEGATIVE_INFINITY,
    };
    if (profileName) {
      const timestampMs = Number.isFinite(observation.timestampMs) ? observation.timestampMs : 0;
      if (timestampMs >= observed.latestTimestampMs) {
        observed.latestName = profileName;
        observed.latestTimestampMs = timestampMs;
      }
    }
    observedNames.set(profileId, observed);
  });

  currentNames.forEach((_name, profileId) => {
    const normalizedId = profileId.trim();
    if (!normalizedId) return;
    const observed = observedNames.get(normalizedId) ?? {
      latestName: '',
      latestTimestampMs: Number.NEGATIVE_INFINITY,
    };
    observedNames.set(normalizedId, observed);
  });

  const normalizedSelectedId = selectedProfileId.trim();
  if (normalizedSelectedId && normalizedSelectedId !== 'all' && !observedNames.has(normalizedSelectedId)) {
    observedNames.set(normalizedSelectedId, {
      latestName: selectedProfileName.trim(),
      latestTimestampMs: Number.POSITIVE_INFINITY,
    });
  }

  const options = Array.from(observedNames.entries()).map(([profileId, observed]) => {
    const currentName = currentNames.get(profileId)?.trim() ?? '';
    const selectedName = profileId === normalizedSelectedId ? selectedProfileName.trim() : '';
    let label = currentName || selectedName || observed.latestName || profileId;
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
