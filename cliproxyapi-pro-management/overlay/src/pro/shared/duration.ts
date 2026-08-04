export type GoDurationUnit = 'ns' | 'us' | 'µs' | 'μs' | 'ms' | 's' | 'm' | 'h';

const DURATION_UNIT_MILLISECONDS: Record<GoDurationUnit, number> = {
  ns: 0.000001,
  us: 0.001,
  µs: 0.001,
  μs: 0.001,
  ms: 1,
  s: 1000,
  m: 60_000,
  h: 3_600_000,
};

const GO_DURATION_PART_PATTERN = /(-?\d+(?:\.\d+)?)(ns|us|µs|μs|ms|s|m|h)/g;

export const parsePositiveGoDuration = (value: string, targetUnit: GoDurationUnit): number | null => {
  const source = value.trim();
  if (!source) return null;
  let milliseconds = 0;
  let cursor = 0;
  let matched = false;
  for (const match of source.matchAll(GO_DURATION_PART_PATTERN)) {
    if (match.index !== cursor) return null;
    milliseconds += Number(match[1]) * DURATION_UNIT_MILLISECONDS[match[2] as GoDurationUnit];
    cursor = match.index + match[0].length;
    matched = true;
  }
  if (!matched || cursor !== source.length || !Number.isFinite(milliseconds) || milliseconds <= 0) {
    return null;
  }
  return milliseconds / DURATION_UNIT_MILLISECONDS[targetUnit];
};

export const serializeGoDuration = (value: number, unit: GoDurationUnit): string =>
  `${Math.round(value * 1000) / 1000}${unit}`;
