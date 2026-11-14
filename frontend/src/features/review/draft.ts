import { type DraftBody, type ReviewQuestion, type ReviewSubmission, type Revision, isManual } from './api.ts';

export interface CorrectedEntry {
  readonly attachmentId: string;
  readonly filename: string;
  readonly sizeBytes: number;
}

export interface DraftItem {
  readonly points: string;
  readonly overriding: boolean;
  readonly overridePoints: string;
  readonly overrideReason: string;
  readonly comment: string;
  readonly files: readonly CorrectedEntry[];
}

export interface Draft {
  readonly feedback: string;
  readonly items: ReadonlyMap<string, DraftItem>;
}

export interface DraftPosition {
  readonly revision: number;
  readonly version: number;
}

export const emptyItem: DraftItem = {
  points: '',
  overriding: false,
  overridePoints: '',
  overrideReason: '',
  comment: '',
  files: [],
};

function pointsField(value: number | null): string {
  return value === null ? '' : String(value);
}

export function baseRevision(submission: ReviewSubmission): Revision | null {
  return submission.draft ?? submission.published;
}

export function draftFrom(submission: ReviewSubmission): Draft {
  const base = baseRevision(submission);
  const items = new Map<string, DraftItem>();
  for (const question of submission.questions) {
    const item = base?.items.get(question.id);
    if (item === undefined) {
      items.set(question.id, emptyItem);
      continue;
    }
    items.set(question.id, {
      points: pointsField(item.points),
      overriding: item.overridePoints !== null,
      overridePoints: pointsField(item.overridePoints),
      overrideReason: item.overrideReason ?? '',
      comment: item.comment,
      files: item.correctedFiles.map((file) => ({
        attachmentId: file.attachmentId,
        filename: file.filename,
        sizeBytes: file.sizeBytes,
      })),
    });
  }
  return { feedback: base?.feedback ?? '', items };
}

export function draftPosition(submission: ReviewSubmission): DraftPosition {
  if (submission.draft !== null) {
    return { revision: submission.draft.revision, version: submission.draft.version };
  }
  return { revision: (submission.published?.revision ?? 0) + 1, version: 0 };
}

const wholeNumber = /^\d{1,9}$/;

export function parsePoints(value: string): number | null {
  const trimmed = value.trim();
  return wholeNumber.test(trimmed) ? Number(trimmed) : null;
}

export function draftBody(questions: readonly ReviewQuestion[], draft: Draft): DraftBody {
  return {
    feedback: draft.feedback,
    items: questions.map((question) => {
      const item = draft.items.get(question.id) ?? emptyItem;
      const override = !isManual(question) && item.overriding ? parsePoints(item.overridePoints) : null;
      return {
        question_id: question.id,
        points: isManual(question) ? parsePoints(item.points) : null,
        override_points: override,
        override_reason: override === null ? null : item.overrideReason,
        comment: item.comment,
        corrected_attachment_ids: question.type === 'file' ? item.files.map((file) => file.attachmentId) : [],
      };
    }),
  };
}

export function withItem(draft: Draft, question: string, item: DraftItem): Draft {
  const items = new Map(draft.items);
  items.set(question, item);
  return { ...draft, items };
}
