import { type ReactElement, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { CheckIcon } from '../../components/ui/icons.tsx';
import { paletteDotClass } from '../../components/ui/palette.ts';
import { type FieldAccess, useStoredValue } from '../../components/ui/serverValue.ts';
import { type GroupColour, groupColours } from './api.ts';

function radiosOf(fieldset: HTMLFieldSetElement): HTMLInputElement[] {
  return [...fieldset.elements].filter(
    (element): element is HTMLInputElement => element instanceof HTMLInputElement && element.type === 'radio',
  );
}

const colourAccess: FieldAccess<HTMLFieldSetElement> = {
  read: (fieldset) => radiosOf(fieldset).find((radio) => radio.checked)?.value ?? '',
  write: (fieldset, colour) => {
    for (const radio of radiosOf(fieldset)) {
      radio.checked = radio.value === colour;
    }
  },
};

export function ColourField({
  defaultColour,
  reloads = 0,
}: {
  readonly defaultColour: GroupColour;
  readonly reloads?: number;
}): ReactElement {
  const { t } = useTranslation();
  const legendId = useId();
  const fieldset = useStoredValue<HTMLFieldSetElement>(defaultColour, reloads, colourAccess);
  const [initialColour] = useState(defaultColour);

  return (
    <fieldset ref={fieldset} aria-labelledby={legendId} className="flex flex-col gap-2">
      <legend id={legendId} className="text-label">
        {t('groups.colour')}
      </legend>
      <div className="flex flex-wrap gap-1">
        {groupColours.map((colour) => {
          const word = t(`colours.${colour}`);
          return (
            <label
              key={colour}
              title={word}
              data-swatch={colour}
              className="relative inline-flex size-11 cursor-pointer items-center justify-center rounded-full desktop:pointer-fine:size-10"
            >
              <input
                type="radio"
                name="colour"
                value={colour}
                defaultChecked={colour === initialColour}
                className="peer absolute inset-0 m-0 size-full cursor-pointer appearance-none rounded-full opacity-0"
              />
              <span
                aria-hidden="true"
                className={`pointer-events-none size-8 rounded-full ${paletteDotClass[colour]} ring-offset-2 ring-offset-surface peer-checked:ring-2 peer-checked:ring-text peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-focus`}
              />
              <span
                aria-hidden="true"
                data-swatch-check
                className="pointer-events-none absolute inset-0 hidden items-center justify-center text-bg peer-checked:flex"
              >
                <CheckIcon />
              </span>
              <span className="sr-only">{word}</span>
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}

export function GroupDot({ colour }: { readonly colour: GroupColour }): ReactElement {
  return <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[colour]}`} />;
}
