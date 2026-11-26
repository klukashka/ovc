import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { Page } from '../../components/ui/Page.tsx';
import { formatDate } from '../../lib/format/dates.ts';
import { isLanguage } from '../../lib/i18n/languages.ts';
import { Markdown } from '../../lib/markdown/Markdown.tsx';
import {
  type LegalDocument,
  type LegalDocumentName,
  type LegalLanguage,
  parseLegalDocument,
} from '../../legal/document.ts';
import englishConsent from '../../legal/en/consent.md?raw';
import englishPrivacy from '../../legal/en/privacy.md?raw';
import englishTerms from '../../legal/en/terms.md?raw';
import russianConsent from '../../legal/ru/consent.md?raw';
import russianPrivacy from '../../legal/ru/privacy.md?raw';
import russianTerms from '../../legal/ru/terms.md?raw';

const sources: Readonly<Record<LegalLanguage, Readonly<Record<LegalDocumentName, string>>>> = {
  ru: { privacy: russianPrivacy, consent: russianConsent, terms: russianTerms },
  en: { privacy: englishPrivacy, consent: englishConsent, terms: englishTerms },
};

export function legalDocument(language: LegalLanguage, name: LegalDocumentName): LegalDocument {
  const parsed = parseLegalDocument(sources[language][name]);
  if (parsed.kind === 'refused') {
    throw new Error(`the legal text ${language}/${name} breaks its rules: ${parsed.problems.join('; ')}`);
  }
  return parsed.document;
}

export function LegalText({ name }: { readonly name: LegalDocumentName }): ReactElement {
  const { t, i18n } = useTranslation();
  const resolved = i18n.resolvedLanguage ?? null;
  const language: LegalLanguage = isLanguage(resolved) ? resolved : 'ru';
  const { metadata, body } = legalDocument(language, name);
  return (
    <>
      <p className="text-small text-text-muted">
        {t('legal.version', {
          version: metadata.version,
          date: formatDate(`${metadata.date}T00:00:00Z`, language, 'UTC'),
        })}
      </p>
      <div className="text-body-phone tablet:text-body">
        <Markdown source={body} />
      </div>
    </>
  );
}

export function PrivacyPage(): ReactElement {
  return (
    <Page width="reading">
      <LegalText name="privacy" />
    </Page>
  );
}

export function ConsentPage(): ReactElement {
  return (
    <Page width="reading">
      <LegalText name="consent" />
    </Page>
  );
}

export function TermsPage(): ReactElement {
  return (
    <Page width="reading">
      <LegalText name="terms" />
    </Page>
  );
}
