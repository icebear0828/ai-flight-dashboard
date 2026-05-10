type WailsWindow = Window & {
  go?: {
    desktop?: {
      App?: {
        OpenSystemLogs?: () => Promise<void> | void;
      };
    };
  };
};

export const wailsWindow = (): WailsWindow => window as WailsWindow;
