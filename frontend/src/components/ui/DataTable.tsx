import type { ReactElement, ReactNode, RefObject } from 'react';

export interface TableColumn {
  readonly key: string;
  readonly label: string;
  readonly secondary?: boolean;
  readonly visuallyHidden?: boolean;
  readonly width?: string;
}

export const rowLinkAttribute = 'data-row-link';

export const rowLinkSelector = `[${rowLinkAttribute}]`;

export const tableRowClass = 'border-t border-border align-middle hover:bg-surface';

export const primaryCellClass = 'px-2 py-3 text-left font-normal [overflow-wrap:anywhere]';

export const cellClass = 'px-2 py-3 [overflow-wrap:anywhere]';

export const secondaryCellClass = 'hidden px-2 py-3 tablet:table-cell [overflow-wrap:anywhere]';

export const narrowOnlyClass = 'tablet:hidden';

export const endCellClass = 'px-2 py-3 text-right [overflow-wrap:anywhere]';

export const rowLinkClass =
  'rounded-sm font-medium text-text underline-offset-2 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent max-tablet:inline-block max-tablet:min-h-11 max-tablet:py-2.5';

function headerClass(column: TableColumn): string {
  const base = `px-2 pb-2 text-left text-label text-text-muted ${column.width ?? ''}`;
  return column.secondary === true ? `${base} hidden tablet:table-cell` : base;
}

export function DataTable({
  label,
  columns,
  bodyRef,
  busy = false,
  children,
}: {
  readonly label: string;
  readonly columns: readonly TableColumn[];
  readonly bodyRef?: RefObject<HTMLTableSectionElement | null>;
  readonly busy?: boolean;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <table aria-label={label} aria-busy={busy} className="w-full table-fixed border-collapse border-b border-border">
      <thead>
        <tr>
          {columns.map((column) => (
            <th key={column.key} scope="col" className={headerClass(column)}>
              {column.visuallyHidden === true ? <span className="sr-only">{column.label}</span> : column.label}
            </th>
          ))}
        </tr>
      </thead>
      <tbody ref={bodyRef}>{children}</tbody>
    </table>
  );
}
