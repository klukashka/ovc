import type { i18n } from 'i18next';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { reportLanguageLoadFailure } from './i18n.ts';
import { languages } from './languages.ts';
import type { FeatureSection, FeatureSections } from './resources.ts';

export type TextLoaders<Section extends FeatureSection> = Readonly<
  Record<(typeof languages)[number], () => Promise<FeatureSections[Section]>>
>;

export interface FeatureTexts {
  readonly ready: boolean;
  readonly pending: boolean;
  readonly failed: boolean;
  readonly retry: () => void;
}

const namespace = 'translation';

function sectionLoaded(translations: i18n, section: FeatureSection): boolean {
  return languages.every((language) => translations.getResource(language, namespace, section) !== undefined);
}

export function useFeatureTexts<Section extends FeatureSection>(
  section: Section,
  loaders: TextLoaders<Section>,
): FeatureTexts {
  const { i18n: translations } = useTranslation();
  const [attempt, setAttempt] = useState(0);
  const [, setLoadedAt] = useState(0);
  const [failed, setFailed] = useState(false);
  const ready = sectionLoaded(translations, section);

  useEffect(() => {
    if (sectionLoaded(translations, section)) {
      return undefined;
    }
    let active = true;
    Promise.all(languages.map(async (language) => ({ language, texts: await loaders[language]() })))
      .then((bundles) => {
        if (!active) {
          return;
        }
        for (const { language, texts } of bundles) {
          translations.addResourceBundle(language, namespace, { [section]: texts }, true, true);
        }
        setFailed(false);
        setLoadedAt((value) => value + 1);
      })
      .catch((error: unknown) => {
        if (active) {
          reportLanguageLoadFailure(error);
          setFailed(true);
        }
      });
    return (): void => {
      active = false;
    };
  }, [translations, section, loaders, attempt]);

  return {
    ready,
    pending: !ready && !failed,
    failed: failed && !ready,
    retry: (): void => {
      setFailed(false);
      setAttempt((value) => value + 1);
    },
  };
}
