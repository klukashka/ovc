import { createContext, useContext } from 'react';

export interface RoomTab {
  readonly navigate: (url: string) => boolean;
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
    navigate: (url): boolean => {
      if (opened.closed) {
        return false;
      }
      try {
        opened.location.replace(url);
      } catch {
        return false;
      }
      return true;
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
