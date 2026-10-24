import type { ReactElement } from 'react';

import { Page } from '../../components/ui/Page.tsx';
import { TutorList } from './TutorList.tsx';

export function MyTutorsPage(): ReactElement {
  return (
    <Page>
      <TutorList withBio />
    </Page>
  );
}
