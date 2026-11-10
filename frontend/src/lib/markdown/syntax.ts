export type InlineNode =
  | { readonly kind: 'text'; readonly text: string }
  | { readonly kind: 'code'; readonly text: string }
  | { readonly kind: 'break' }
  | { readonly kind: 'emphasis'; readonly children: readonly InlineNode[] }
  | { readonly kind: 'strong'; readonly children: readonly InlineNode[] }
  | { readonly kind: 'link'; readonly href: string; readonly children: readonly InlineNode[] };

export type ListItem = readonly InlineNode[];

export type BlockNode =
  | { readonly kind: 'paragraph'; readonly content: readonly InlineNode[] }
  | { readonly kind: 'code_block'; readonly text: string }
  | { readonly kind: 'unordered_list'; readonly items: readonly ListItem[] }
  | { readonly kind: 'ordered_list'; readonly start: number; readonly items: readonly ListItem[] };

export interface StepCounter {
  steps: number;
}
