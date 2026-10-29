import { shallowRef } from "vue";

// Keep file clipboard metadata inside the app; never paste it onto another host.
export const remoteFileClipboard = shallowRef<{
  server: string;
  path: string;
  name: string;
  cut: boolean;
  sudo: boolean;
  items?: { path: string; name: string; directory: boolean }[];
} | null>(null);
