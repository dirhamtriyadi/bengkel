export {};

declare global {
  interface Window {
    snap?: {
      pay: (token: string, callbacks: {
        onSuccess: (result?: unknown) => void;
        onPending: (result?: unknown) => void;
        onError: (result?: unknown) => void;
        onClose: () => void;
      }) => void;
    };
  }
}
