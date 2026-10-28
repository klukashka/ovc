import type { components } from '../../lib/api/schema.ts';

export type PaletteColour = components['schemas']['MyTutor']['colour'];

export const paletteDotClass: Readonly<Record<PaletteColour, string>> = {
  clay: 'bg-clay',
  slate: 'bg-slate',
  moss: 'bg-moss',
  plum: 'bg-plum',
  ochre: 'bg-ochre',
  teal: 'bg-teal',
  rust: 'bg-rust',
  ink: 'bg-ink',
};

export const paletteEdgeClass: Readonly<Record<PaletteColour, string>> = {
  clay: 'border-l-clay',
  slate: 'border-l-slate',
  moss: 'border-l-moss',
  plum: 'border-l-plum',
  ochre: 'border-l-ochre',
  teal: 'border-l-teal',
  rust: 'border-l-rust',
  ink: 'border-l-ink',
};
