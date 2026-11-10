import type { AssignmentTexts } from './texts.ts';

export const assignmentTexts: AssignmentTexts = {
  question: 'Вопрос {number}',
  answer: 'Ваш ответ',
  lengthHint: 'Не больше {count, plural, one {# символа} few {# символов} many {# символов} other {# символа}}',
  fileHint: '{kinds}; не больше {count, plural, one {# файла} few {# файлов} many {# файлов} other {# файла}}',
  attach: 'Прикрепить файлы',
  kinds: { images: 'изображения', pdf: 'PDF', office: 'офисные документы', audio: 'аудио' },
  material: 'Материал',
  download: 'Скачать',
};
