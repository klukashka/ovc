import { createContext, useContext } from 'react';

export interface RoomTab {
  readonly navigate: (url: string) => void;
  readonly close: () => void;
}

export type OpenRoomTab = () => RoomTab | null;

export const openBrowserRoomTab: OpenRoomTab = () => {
  const opened = window.open('about:blank', '_blank');
  if (opened === null) {
    return null;
  }
  opened.opener = null;
  return {
    navigate: (url): void => {
      opened.location.replace(url);
    },
    close: (): void => {
      opened.close();
    },
  };
};

export const RoomTabContext = createContext<OpenRoomTab>(openBrowserRoomTab);

export function useOpenRoomTab(): OpenRoomTab {
  return useContext(RoomTabContext);
}
