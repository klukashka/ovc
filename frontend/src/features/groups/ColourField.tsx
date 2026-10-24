import { type ReactElement, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';

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
      <div className="flex flex-wrap gap-x-4 gap-y-1">
        {groupColours.map((colour) => (
          <label key={colour} className="inline-flex min-h-11 items-center gap-2 text-small">
            <input
              type="radio"
              name="colour"
              value={colour}
              defaultChecked={colour === initialColour}
              className="size-4 accent-accent"
            />
            <span aria-hidden="true" className={`size-3 shrink-0 rounded-full ${paletteDotClass[colour]}`} />
            {t(`colours.${colour}`)}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

export function GroupDot({ colour }: { readonly colour: GroupColour }): ReactElement {
  return <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[colour]}`} />;
}
