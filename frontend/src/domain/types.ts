import type {
  AddFilesResult,
  CompressResult,
  ItemInfo,
} from "@/bindings/picpress/internal/imagepress/models";

export type { AddFilesResult, CompressResult, ItemInfo };

/** One image in the session, together with its latest compression result. */
export interface ImageItem {
  info: ItemInfo;
  result: CompressResult | null;
  updating: boolean;
  error: string;
  savedPath: string;
}

export interface Progress {
  id: string;
  index: number;
  total: number;
}

export type ToastKind = "info" | "success" | "error" | "warn";

export interface ToastAction {
  label: string;
  onClick: () => void;
}

export interface Toast {
  id: number;
  message: string;
  kind: ToastKind;
  action?: ToastAction;
}

export interface Summary {
  count: number;
  resultCount: number;
  original: number;
  output: number;
  saved: number;
  percent: number;
  bigger: boolean;
}
