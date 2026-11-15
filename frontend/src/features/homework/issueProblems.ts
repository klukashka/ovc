import type { TFunction } from 'i18next';

import type { ActionState } from '../../components/ui/action.ts';
import { refusalText } from '../../components/ui/Refusal.tsx';
import type { ApiError } from '../../lib/api/client.ts';
import { builderAddress } from './api.ts';
import type { ChosenTemplate } from './TemplatePicker.tsx';

export type Problem =
  | { readonly kind: 'alert'; readonly message: string; readonly builderLink: string | null }
  | { readonly kind: 'confirmation' }
  | { readonly kind: 'deadline' }
  | { readonly kind: 'lesson' };

export function problemOf(t: TFunction, state: ActionState, template: ChosenTemplate | null): Problem | null {
  if (state.kind !== 'refused') {
    return null;
  }
  const error: ApiError = state.error;
  const alert = (message: string, builderLink: string | null = null): Problem => ({
    kind: 'alert',
    message,
    builderLink,
  });
  switch (error.code) {
    case 'not_editable':
      return alert(t('homework.refusals.notEditable'), template === null ? null : builderAddress(template.id));
    case 'relationship_not_active':
      return alert(t('homework.refusals.relationshipNotActive'));
    case 'audience_not_active':
      return alert(t('homework.refusals.audienceNotActive'));
    case 'not_found':
      return alert(t('homework.refusals.notFound'));
    case 'email_not_verified':
      return { kind: 'confirmation' };
    default:
      break;
  }
  if (error.fieldCode('deadline_at') !== undefined) {
    return { kind: 'deadline' };
  }
  if (error.fieldCode('lesson_id') === 'too_many') {
    return { kind: 'lesson' };
  }
  if (error.fieldCode('body') === 'too_many') {
    return alert(t('homework.refusals.storageLimit'));
  }
  return alert(refusalText(t, error, state.retryAfter, undefined));
}
