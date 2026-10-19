import { useAtomSet, useAtomValue } from "@effect/atom-react";
import * as Effect from "effect/Effect";
import { useCallback, useEffect, useRef } from "react";
import { errorText, baseName } from "@/domain/format";
import { applyResults, markUpdating, mergeInfos } from "@/domain/session";
import { DEFAULT_SETTINGS, saveSettings, toGoOptions, type Settings } from "@/domain/settings";
import type { AddFilesResult, ImageItem, Progress, Summary } from "@/domain/types";
import * as wails from "@/services/wails";
import {
  compareIdAtom,
  compressingAtom,
  itemsAtom,
  pickingAtom,
  progressAtom,
  savingAtom,
  settingsAtom,
  summaryAtom,
} from "@/state/atoms";
import { useToast } from "./useToast";

export interface SessionApi {
  items: ImageItem[];
  settings: Settings;
  summary: Summary;
  compressing: boolean;
  picking: boolean;
  saving: boolean;
  progress: Progress | null;
  compareId: string | null;
  addPaths: (paths: string[]) => Promise<void>;
  pickFiles: () => Promise<void>;
  removeItem: (id: string) => Promise<void>;
  clearAll: () => Promise<void>;
  saveOne: (id: string) => Promise<void>;
  saveAll: () => Promise<void>;
  reveal: (path: string) => void;
  openCompare: (id: string) => void;
  closeCompare: () => void;
  updateSettings: (patch: Partial<Settings>) => void;
  resetSettings: () => void;
  compressNow: () => Promise<void>;
}

export function useSession(): SessionApi {
  const items = useAtomValue(itemsAtom);
  const settings = useAtomValue(settingsAtom);
  const summary = useAtomValue(summaryAtom);
  const compressing = useAtomValue(compressingAtom);
  const picking = useAtomValue(pickingAtom);
  const saving = useAtomValue(savingAtom);
  const progress = useAtomValue(progressAtom);
  const compareId = useAtomValue(compareIdAtom);

  const setItems = useAtomSet(itemsAtom);
  const setSettings = useAtomSet(settingsAtom);
  const setCompressing = useAtomSet(compressingAtom);
  const setPicking = useAtomSet(pickingAtom);
  const setSaving = useAtomSet(savingAtom);
  const setProgress = useAtomSet(progressAtom);
  const setCompareId = useAtomSet(compareIdAtom);
  const toast = useToast();

  const itemsRef = useRef(items);
  const settingsRef = useRef(settings);
  const runningRef = useRef(false);
  const dirtyRef = useRef(false);
  const pickingRef = useRef(false);
  const savingRef = useRef(false);
  const compressRef = useRef<() => Promise<void>>(async () => {});

  useEffect(() => {
    itemsRef.current = items;
  }, [items]);

  useEffect(() => {
    settingsRef.current = settings;
  }, [settings]);

  useEffect(() => {
    saveSettings(settings);
  }, [settings]);

  const commitItems = useCallback(
    (next: ImageItem[]) => {
      itemsRef.current = next;
      setItems(next);
    },
    [setItems],
  );

  const compressNow = useCallback(async () => {
    if (itemsRef.current.length === 0) return;
    if (runningRef.current) {
      dirtyRef.current = true;
      return;
    }

    runningRef.current = true;
    setCompressing(true);
    setProgress(null);

    try {
      do {
        dirtyRef.current = false;
        const snapshot = itemsRef.current;
        if (snapshot.length === 0) break;

        commitItems(markUpdating(snapshot));
        try {
          const results =
            (await Effect.runPromise(wails.compress(toGoOptions(settingsRef.current)))) ?? [];
          commitItems(applyResults(itemsRef.current, results));
        } catch (error) {
          const message = errorText(error);
          commitItems(
            itemsRef.current.map((item) =>
              item.updating
                ? { ...item, updating: false, error: item.result ? "" : message }
                : item,
            ),
          );
          toast(`压缩失败：${message}`, "error");
          break;
        }
      } while (dirtyRef.current);
    } finally {
      runningRef.current = false;
      setCompressing(false);
      setProgress(null);
      if (dirtyRef.current) {
        dirtyRef.current = false;
        window.setTimeout(() => {
          void compressRef.current();
        }, 0);
      }
    }
  }, [commitItems, setCompressing, setProgress, toast]);

  useEffect(() => {
    compressRef.current = compressNow;
  }, [compressNow]);

  const ingest = useCallback(
    (result: AddFilesResult) => {
      const incoming = result.items ?? [];
      const next = mergeInfos(itemsRef.current, incoming);
      commitItems(next);

      const errors = result.errors ?? [];
      if (errors.length > 0) {
        const first = errors[0];
        const name = first ? baseName(first.path) : "文件";
        const extra = errors.length > 1 ? `，另有 ${errors.length - 1} 个失败` : "";
        toast(`${name}：${first?.message ?? "添加失败"}${extra}`, "error");
      }

      if (next.length > 0) {
        void compressNow();
      }
    },
    [commitItems, compressNow, toast],
  );

  const addPaths = useCallback(
    async (paths: string[]) => {
      if (!paths || paths.length === 0) return;
      try {
        const result = await Effect.runPromise(wails.addFiles(paths));
        ingest(result);
      } catch (error) {
        toast(`添加图片失败：${errorText(error)}`, "error");
      }
    },
    [ingest, toast],
  );

  const pickFiles = useCallback(async () => {
    if (pickingRef.current) return;
    pickingRef.current = true;
    setPicking(true);
    try {
      const result = await Effect.runPromise(wails.pickFiles());
      ingest(result);
    } catch (error) {
      toast(`打开文件选择器失败：${errorText(error)}`, "error");
    } finally {
      pickingRef.current = false;
      setPicking(false);
    }
  }, [ingest, setPicking, toast]);

  const removeItem = useCallback(
    async (id: string) => {
      const item = itemsRef.current.find((entry) => entry.info.id === id);
      try {
        await Effect.runPromise(wails.remove(id));
      } catch (error) {
        toast(`移除失败：${errorText(error)}`, "error");
        return;
      }
      const next = itemsRef.current.filter((entry) => entry.info.id !== id);
      commitItems(next);
      if (next.length === 0) {
        dirtyRef.current = false;
        setCompressing(false);
        setProgress(null);
      }
      if (item) toast(`已移除 ${item.info.name}`);
    },
    [commitItems, setCompressing, setProgress, toast],
  );

  const clearAll = useCallback(async () => {
    if (itemsRef.current.length === 0) return;
    try {
      await Effect.runPromise(wails.clear());
    } catch (error) {
      toast(`清空失败：${errorText(error)}`, "error");
      return;
    }
    dirtyRef.current = false;
    commitItems([]);
    setCompressing(false);
    setProgress(null);
    toast("列表已清空");
  }, [commitItems, setCompressing, setProgress, toast]);

  const saveOne = useCallback(
    async (id: string) => {
      const item = itemsRef.current.find((entry) => entry.info.id === id);
      if (!item?.result || item.updating) return;

      try {
        const path = await Effect.runPromise(wails.saveFile(id));
        if (!path) return;
        commitItems(
          itemsRef.current.map((entry) =>
            entry.info.id === id ? { ...entry, savedPath: path } : entry,
          ),
        );
        toast(`已保存 ${baseName(path)}`, "success", {
          label: "在 Finder 中显示",
          onClick: () => {
            void Effect.runPromise(wails.revealPath(path)).catch(() => {});
          },
        });
      } catch (error) {
        toast(`保存失败：${errorText(error)}`, "error");
      }
    },
    [commitItems, toast],
  );

  const saveAll = useCallback(async () => {
    if (savingRef.current || itemsRef.current.length === 0) return;
    if (!itemsRef.current.some((item) => item.result && !item.error)) return;
    savingRef.current = true;
    setSaving(true);
    try {
      const result = await Effect.runPromise(wails.saveAll([]));
      if (!result.directory) return;

      const saved = result.saved ?? [];
      if (saved.length === 0) {
        toast("没有可保存的图片", "warn");
        return;
      }

      commitItems(
        itemsRef.current.map((item) =>
          item.result ? { ...item, savedPath: result.directory } : item,
        ),
      );
      toast(
        saved.length === itemsRef.current.length
          ? `${saved.length} 张图片已保存`
          : `${saved.length} 张图片已保存，${itemsRef.current.length - saved.length} 张失败`,
        saved.length === itemsRef.current.length ? "success" : "warn",
        {
          label: "打开文件夹",
          onClick: () => {
            void Effect.runPromise(wails.revealPath(result.directory)).catch(() => {});
          },
        },
      );
      if (result.errors && result.errors.length > 0) {
        toast(result.errors[0], "error");
      }
    } catch (error) {
      toast(`保存失败：${errorText(error)}`, "error");
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }, [commitItems, setSaving, toast]);

  const reveal = useCallback((path: string) => {
    void Effect.runPromise(wails.revealPath(path)).catch(() => {});
  }, []);

  const openCompare = useCallback(
    (id: string) => {
      setCompareId(id);
    },
    [setCompareId],
  );

  const closeCompare = useCallback(() => {
    setCompareId(null);
  }, [setCompareId]);

  const updateSettings = useCallback(
    (patch: Partial<Settings>) => {
      setSettings((previous) => ({ ...previous, ...patch }));
    },
    [setSettings],
  );

  const resetSettings = useCallback(() => {
    setSettings(DEFAULT_SETTINGS);
    toast("已恢复默认设置");
  }, [setSettings, toast]);

  useEffect(() => {
    if (itemsRef.current.length === 0) return;
    const timer = window.setTimeout(() => {
      void compressNow();
    }, 320);
    return () => window.clearTimeout(timer);
  }, [settings, compressNow]);

  return {
    items,
    settings,
    summary,
    compressing,
    picking,
    saving,
    progress,
    compareId,
    addPaths,
    pickFiles,
    removeItem,
    clearAll,
    saveOne,
    saveAll,
    reveal,
    openCompare,
    closeCompare,
    updateSettings,
    resetSettings,
    compressNow,
  };
}
