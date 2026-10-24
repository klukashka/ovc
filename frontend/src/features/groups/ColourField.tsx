import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { paletteDotClass } from '../../components/ui/palette.ts';
import { type GroupColour, groupColours } from './api.ts';

export function ColourField({ defaultColour }: { readonly defaultColour: GroupColour }): ReactElement {
  const { t } = useTranslation();
  const legendId = useId();
  return (
    <fieldset aria-labelledby={legendId} className="flex flex-col gap-2">
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
              defaultChecked={colour === defaultColour}
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
