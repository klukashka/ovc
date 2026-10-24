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
