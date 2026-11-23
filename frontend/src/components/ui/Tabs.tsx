import type { ReactElement, ReactNode } from 'react';
import { Link } from 'react-router';

const tabClass = 'inline-flex min-h-11 items-center border-b-2 px-1 text-small';
const currentTabClass = `${tabClass} border-accent font-medium text-text`;
const otherTabClass = `${tabClass} border-transparent text-text-muted hover:text-text`;

export interface RouteTab<K extends string> {
  readonly key: K;
  readonly to: string;
  readonly label: ReactNode;
}

export function RouteTabs<K extends string>({
  label,
  tabs,
  current,
  inFilterRow = false,
}: {
  readonly label: string;
  readonly tabs: readonly RouteTab<K>[];
  readonly current: K | null;
  readonly inFilterRow?: boolean;
}): ReactElement {
  return (
    <nav
      aria-label={label}
      className={inFilterRow ? 'shrink-0 border-b border-border desktop:border-b-0' : 'border-b border-border'}
    >
      <ul className="flex flex-wrap gap-6">
        {tabs.map((tab) => (
          <li key={tab.key}>
            <Link
              to={tab.to}
              aria-current={tab.key === current ? 'page' : undefined}
              className={tab.key === current ? currentTabClass : otherTabClass}
            >
              {tab.label}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}

const segmentClass =
  'inline-flex min-h-11 max-w-full min-w-0 items-center justify-center gap-2 rounded-sm border border-transparent px-3 text-small text-text-muted hover:text-text aria-pressed:border-border-strong aria-pressed:bg-field aria-pressed:font-medium aria-pressed:text-text desktop:pointer-fine:min-h-10';

const fillingSegmentClass = `${segmentClass} grow tablet:grow-0`;

export interface Segment<V extends string> {
  readonly value: V;
  readonly label: ReactNode;
}

export type SegmentedLabel = { readonly label: string } | { readonly labelledBy: string };

export function SegmentedControl<V extends string>({
  name,
  segments,
  chosen,
  fill = false,
  onChoose,
}: {
  readonly name: SegmentedLabel;
  readonly segments: readonly Segment<V>[];
  readonly chosen: V;
  readonly fill?: boolean;
  readonly onChoose: (value: V) => void;
}): ReactElement {
  return (
    <div
      role="group"
      {...('label' in name ? { 'aria-label': name.label } : { 'aria-labelledby': name.labelledBy })}
      className="inline-flex flex-wrap gap-1 rounded-md bg-hover p-1"
    >
      {segments.map((segment) => (
        <button
          key={segment.value}
          type="button"
          aria-pressed={segment.value === chosen}
          onClick={() => {
            onChoose(segment.value);
          }}
          className={fill ? fillingSegmentClass : segmentClass}
        >
          {segment.label}
        </button>
      ))}
    </div>
  );
}
