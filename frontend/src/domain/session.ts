import type { CompressResult, ImageItem, ItemInfo, Summary } from "./types";

export function mergeInfos(previous: ImageItem[], incoming: ItemInfo[] | null): ImageItem[] {
  const nextById = new Map((incoming ?? []).map((info) => [info.id, info]));
  const merged = previous.map((item) => {
    const info = nextById.get(item.info.id);
    return info ? { ...item, info } : item;
  });
  const known = new Set(merged.map((item) => item.info.id));
  for (const info of incoming ?? []) {
    if (known.has(info.id)) continue;
    merged.push({
      info,
      result: null,
      updating: false,
      error: "",
      savedPath: "",
    });
  }
  return merged;
}

export function markUpdating(items: ImageItem[]): ImageItem[] {
  return items.map((item) => ({ ...item, updating: true, error: "" }));
}

export function applyResults(items: ImageItem[], results: CompressResult[]): ImageItem[] {
  const byId = new Map(results.map((result) => [result.id, result]));
  return items.map((item) => {
    const result = byId.get(item.info.id);
    if (!result) return item;
    if (result.error) {
      return { ...item, updating: false, result: null, error: result.error };
    }
    return {
      ...item,
      updating: false,
      result,
      error: "",
      savedPath: "",
    };
  });
}

export function summarize(items: ImageItem[]): Summary {
  const finished = items.filter((item) => item.result && !item.error);
  const original = finished.reduce((sum, item) => sum + (item.info.bytes || 0), 0);
  const output = finished.reduce((sum, item) => sum + (item.result?.bytes ?? 0), 0);
  const saved = original - output;
  const percent = original > 0 ? (saved / original) * 100 : 0;
  return {
    count: items.length,
    resultCount: finished.length,
    original,
    output,
    saved,
    percent,
    bigger: saved < 0 && finished.length > 0,
  };
}

export function canSave(summary: Summary, compressing: boolean, saving: boolean): boolean {
  return !compressing && !saving && summary.resultCount > 0;
}
