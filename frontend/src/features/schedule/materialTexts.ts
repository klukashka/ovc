import { useTranslation } from 'react-i18next';

import { type Language, isLanguage } from '../../lib/i18n/languages.ts';

export interface MaterialTexts {
  readonly materials: string;
  readonly noMaterials: string;
  readonly attachMaterial: string;
  readonly detach: string;
  readonly noLibraryMaterials: string;
  readonly materialsLimit: string;
  readonly materialGone: string;
  readonly libraryRoot: string;
  readonly libraryPath: string;
  readonly libraryFolders: string;
  readonly librarySearch: string;
}

const materialTexts: Readonly<Record<Language, MaterialTexts>> = {
  en: {
    materials: 'Materials',
    noMaterials: 'No materials',
    attachMaterial: 'Attach material',
    detach: 'Detach',
    noLibraryMaterials: 'No materials here',
    materialsLimit: 'A lesson holds at most 20 materials.',
    materialGone: 'A chosen material is no longer in the library, or this student or group can no longer get lessons.',
    libraryRoot: 'Library',
    libraryPath: 'Folder path',
    libraryFolders: 'Folders',
    librarySearch: 'Search the library',
  },
  ru: {
    materials: 'Материалы',
    noMaterials: 'Материалов нет',
    attachMaterial: 'Прикрепить материал',
    detach: 'Открепить',
    noLibraryMaterials: 'Здесь нет материалов',
    materialsLimit: 'К уроку можно прикрепить не больше 20 материалов.',
    materialGone:
      'Выбранного материала больше нет в библиотеке, или этот ученик или группа больше недоступны для уроков.',
    libraryRoot: 'Библиотека',
    libraryPath: 'Путь к папке',
    libraryFolders: 'Папки',
    librarySearch: 'Поиск по библиотеке',
  },
};

export function useMaterialTexts(): MaterialTexts {
  const { i18n } = useTranslation();
  const language = i18n.resolvedLanguage ?? i18n.language;
  if (!isLanguage(language)) {
    throw new Error('Unsupported language for the lesson material texts');
  }
  return materialTexts[language];
}
