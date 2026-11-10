import type { components } from '../../lib/api/schema.ts';

export type TemplateBlockResponse = components['schemas']['TemplateBlock'];
export type TemplateBlockInput = components['schemas']['TemplateBlockInput'];
export type TemplateFile = components['schemas']['TemplateFile'];
export type TemplateMaterial = components['schemas']['TemplateMaterial'];

export const blockTypes = [
  'instruction',
  'material',
  'single_choice',
  'multiple_choice',
  'short_text',
  'short_number',
  'long_text',
  'file',
] as const;

export type BlockType = (typeof blockTypes)[number];

export const fileKinds = ['images', 'pdf', 'office', 'audio'] as const;

export type FileKind = (typeof fileKinds)[number];

export type Scoring = 'all_or_nothing' | 'partial';

export const limits = {
  title: 200,
  text: 10_000,
  option: 500,
  acceptedAnswer: 500,
  decimal: 64,
  blocks: 100,
  options: 10,
  acceptedAnswers: 20,
  images: 3,
  points: { min: 0, max: 100 },
  maxLength: { min: 1, max: 20_000 },
  maxFiles: { min: 1, max: 5 },
} as const;

export interface ChoiceOption {
  readonly id: string;
  readonly text: string;
  readonly correct: boolean;
}

interface QuestionFields {
  readonly id: string;
  readonly prompt: string;
  readonly imageIds: readonly string[];
  readonly points: number;
  readonly explanation: string;
}

export type Block =
  | { readonly type: 'instruction'; readonly id: string; readonly text: string }
  | { readonly type: 'material'; readonly id: string; readonly materialId: string }
  | (QuestionFields & { readonly type: 'single_choice'; readonly options: readonly ChoiceOption[] })
  | (QuestionFields & {
      readonly type: 'multiple_choice';
      readonly options: readonly ChoiceOption[];
      readonly scoring: Scoring;
    })
  | (QuestionFields & { readonly type: 'short_text'; readonly acceptedAnswers: readonly string[] })
  | (QuestionFields & { readonly type: 'short_number'; readonly value: string; readonly tolerance: string })
  | (QuestionFields & { readonly type: 'long_text'; readonly maxLength: number })
  | (QuestionFields & {
      readonly type: 'file';
      readonly allowedKinds: readonly FileKind[];
      readonly maxFiles: number;
    });

export type QuestionBlock = Exclude<Block, { readonly type: 'instruction' } | { readonly type: 'material' }>;

export type ChoiceBlock = Extract<Block, { readonly type: 'single_choice' | 'multiple_choice' }>;

export function isQuestion(block: Block): block is QuestionBlock {
  return block.type !== 'instruction' && block.type !== 'material';
}

export function isChoice(block: Block): block is ChoiceBlock {
  return block.type === 'single_choice' || block.type === 'multiple_choice';
}

export class MalformedBlockError extends Error {
  constructor(field: string) {
    super(`A template block arrived without a valid ${field}`);
    this.name = 'MalformedBlockError';
  }
}

function required<Value>(value: Value | undefined, field: string): Value {
  if (value === undefined) {
    throw new MalformedBlockError(field);
  }
  return value;
}

function isFileKind(value: string): value is FileKind {
  return fileKinds.some((kind) => kind === value);
}

function parsedOptions(block: TemplateBlockResponse): readonly ChoiceOption[] {
  return required(block.options, 'options').map((option) => ({
    id: option.id,
    text: option.text,
    correct: required(option.correct, 'correct'),
  }));
}

function questionFields(block: TemplateBlockResponse): QuestionFields {
  return {
    id: block.id,
    prompt: required(block.prompt, 'prompt'),
    imageIds: block.image_ids ?? [],
    points: required(block.points, 'points'),
    explanation: block.explanation ?? '',
  };
}

export function parseBlock(block: TemplateBlockResponse): Block {
  switch (block.type) {
    case 'instruction':
      return { type: 'instruction', id: block.id, text: required(block.text, 'text') };
    case 'material':
      return { type: 'material', id: block.id, materialId: required(block.material_id, 'material_id') };
    case 'single_choice':
      return { ...questionFields(block), type: 'single_choice', options: parsedOptions(block) };
    case 'multiple_choice':
      return {
        ...questionFields(block),
        type: 'multiple_choice',
        options: parsedOptions(block),
        scoring: required(block.scoring, 'scoring'),
      };
    case 'short_text':
      return { ...questionFields(block), type: 'short_text', acceptedAnswers: block.accepted_answers ?? [] };
    case 'short_number':
      return {
        ...questionFields(block),
        type: 'short_number',
        value: required(block.value, 'value'),
        tolerance: required(block.tolerance, 'tolerance'),
      };
    case 'long_text':
      return { ...questionFields(block), type: 'long_text', maxLength: required(block.max_length, 'max_length') };
    case 'file': {
      const kinds = required(block.allowed_kinds, 'allowed_kinds');
      if (!kinds.every(isFileKind)) {
        throw new MalformedBlockError('allowed_kinds');
      }
      return {
        ...questionFields(block),
        type: 'file',
        allowedKinds: kinds.filter(isFileKind),
        maxFiles: required(block.max_files, 'max_files'),
      };
    }
  }
}

function questionInput(block: QuestionBlock): TemplateBlockInput {
  return {
    id: block.id,
    type: block.type,
    prompt: block.prompt,
    image_ids: [...block.imageIds],
    points: block.points,
    explanation: block.explanation,
  };
}

function optionInputs(options: readonly ChoiceOption[]): components['schemas']['TemplateOption'][] {
  return options.map((option) => ({ id: option.id, text: option.text, correct: option.correct }));
}

export function blockInput(block: Block): TemplateBlockInput {
  switch (block.type) {
    case 'instruction':
      return { id: block.id, type: block.type, text: block.text };
    case 'material':
      return { id: block.id, type: block.type, material_id: block.materialId };
    case 'single_choice':
      return { ...questionInput(block), options: optionInputs(block.options) };
    case 'multiple_choice':
      return { ...questionInput(block), options: optionInputs(block.options), scoring: block.scoring };
    case 'short_text':
      return { ...questionInput(block), accepted_answers: [...block.acceptedAnswers] };
    case 'short_number':
      return { ...questionInput(block), value: block.value, tolerance: block.tolerance };
    case 'long_text':
      return { ...questionInput(block), max_length: block.maxLength };
    case 'file':
      return { ...questionInput(block), allowed_kinds: [...block.allowedKinds], max_files: block.maxFiles };
  }
}

export type NewBlockType = Exclude<BlockType, 'material'>;

const defaultPoints = 1;
const defaultMaxLength = 2000;

function newOption(newId: () => string): ChoiceOption {
  return { id: newId(), text: '', correct: false };
}

function newQuestion(newId: () => string): QuestionFields {
  return { id: newId(), prompt: '', imageIds: [], points: defaultPoints, explanation: '' };
}

export function newBlock(type: NewBlockType, newId: () => string): Block {
  switch (type) {
    case 'instruction':
      return { type, id: newId(), text: '' };
    case 'single_choice':
      return { ...newQuestion(newId), type, options: [newOption(newId), newOption(newId)] };
    case 'multiple_choice':
      return {
        ...newQuestion(newId),
        type,
        options: [newOption(newId), newOption(newId)],
        scoring: 'all_or_nothing',
      };
    case 'short_text':
      return { ...newQuestion(newId), type, acceptedAnswers: [] };
    case 'short_number':
      return { ...newQuestion(newId), type, value: '', tolerance: '0' };
    case 'long_text':
      return { ...newQuestion(newId), type, maxLength: defaultMaxLength };
    case 'file':
      return { ...newQuestion(newId), type, allowedKinds: ['images', 'pdf'], maxFiles: 1 };
  }
}

export function newMaterialBlock(materialId: string, newId: () => string): Block {
  return { type: 'material', id: newId(), materialId };
}

export function duplicateBlock(block: Block, newId: () => string): Block {
  if (isChoice(block)) {
    return { ...block, id: newId(), options: block.options.map((option) => ({ ...option, id: newId() })) };
  }
  return { ...block, id: newId() };
}

export function cutToCodePoints(text: string, limit: number): string {
  const points = Array.from(text);
  return points.length <= limit ? text : points.slice(0, limit).join('');
}

export function clampInteger(value: number, range: { readonly min: number; readonly max: number }): number {
  return Math.min(range.max, Math.max(range.min, Math.trunc(value)));
}

export function parseInteger(text: string, range: { readonly min: number; readonly max: number }): number | null {
  if (!/^[0-9]{1,6}$/.test(text.trim())) {
    return null;
  }
  const value = Number(text.trim());
  return value < range.min || value > range.max ? null : value;
}

export function moveBlock(blocks: readonly Block[], index: number, offset: -1 | 1): readonly Block[] {
  const target = index + offset;
  const moving = blocks[index];
  const other = blocks[target];
  if (moving === undefined || other === undefined) {
    return blocks;
  }
  const moved = [...blocks];
  moved[index] = other;
  moved[target] = moving;
  return moved;
}

export function placeBlock(blocks: readonly Block[], from: number, to: number): readonly Block[] {
  const moving = blocks[from];
  if (moving === undefined || to < 0 || to >= blocks.length || from === to) {
    return blocks;
  }
  const rest = blocks.filter((_, index) => index !== from);
  return [...rest.slice(0, to), moving, ...rest.slice(to)];
}
