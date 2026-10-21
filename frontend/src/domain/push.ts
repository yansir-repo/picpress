import type { PushSnapshot } from "@/bindings/picpress/models";
import type { FileUpdate, Plan, Skip } from "@/bindings/picpress/internal/push/models";

export type { FileUpdate, Plan, PushSnapshot, Skip };

/** Statuses after which a file will not change again in this run. */
const SETTLED = new Set(["uploaded", "skipped-duplicate", "failed"]);

export const STATUS_LABELS: Record<string, string> = {
  waiting: "等待",
  compressing: "压缩中",
  uploading: "上传中",
  uploaded: "完成",
  "skipped-duplicate": "重复跳过",
  failed: "失败",
};

export const REASON_LABELS: Record<string, string> = {
  unsupported: "格式暂不支持",
  "too-large": "超过大小上限",
  empty: "空文件或空文件夹",
  hidden: "隐藏或系统文件",
  symlink: "快捷方式",
  "name-conflict-renamed": "云端名称已调整",
  "decode-error": "无法解码",
  "read-error": "读不到",
  network: "网络",
  auth: "没有权限",
};

/** Applies a batch of streamed file updates to a snapshot in O(n + k). */
export function mergeFiles(snapshot: PushSnapshot, updates: FileUpdate[]): PushSnapshot {
  if (updates.length === 0) return snapshot;
  const bySource = new Map(updates.map((update) => [update.source, update]));
  const next = (snapshot.files ?? []).map((file) => {
    const update = bySource.get(file.source);
    if (!update) return file;
    bySource.delete(file.source);
    return update;
  });
  next.push(...bySource.values());
  return { ...snapshot, files: next };
}

export interface Progress {
  settled: number;
  total: number;
  sent: number;
  bytes: number;
}

function progressOf(files: FileUpdate[]): Progress {
  const progress: Progress = { settled: 0, total: files.length, sent: 0, bytes: 0 };
  for (const file of files) {
    if (SETTLED.has(file.status)) progress.settled += 1;
    progress.sent += file.status === "skipped-duplicate" ? file.total : file.sent;
    progress.bytes += file.total;
  }
  return progress;
}

export function overallProgress(files: FileUpdate[]): Progress {
  return progressOf(files);
}

/** Progress per dropped top-level folder, in drop order. */
export function rootProgress(
  files: FileUpdate[],
  roots: string[],
): Array<{ root: string } & Progress> {
  return roots.map((root) => ({
    root,
    ...progressOf(
      files.filter((file) => file.source === root || file.source.startsWith(`${root}/`)),
    ),
  }));
}

/** Plan skips that are real skips (renames still upload). */
export function realSkips(plan: Plan): Skip[] {
  return (plan.skipped ?? []).filter((skip) => skip.reason !== "name-conflict-renamed");
}

export function renames(plan: Plan): Skip[] {
  return (plan.skipped ?? []).filter((skip) => skip.reason === "name-conflict-renamed");
}

export function isActive(phase: string): boolean {
  return phase === "running" || phase === "waiting-network";
}
