import { describe, expect, it } from "@effect/vitest";
import type { FileUpdate, PushSnapshot } from "./push";
import { mergeFiles, overallProgress, realSkips, rootProgress } from "./push";

const file = (source: string, status: string, sent = 0, total = 10): FileUpdate =>
  ({ source, key: source, kind: "image", status, sent, total }) as FileUpdate;

describe("push domain", () => {
  it("merges updates by source, keeping order", () => {
    let snap = { files: [] } as unknown as PushSnapshot;
    snap = mergeFiles(snap, [file("a/1.jpg", "waiting"), file("a/2.jpg", "waiting")]);
    snap = mergeFiles(snap, [file("a/1.jpg", "uploaded", 10)]);
    expect(snap.files?.map((f) => `${f.source}:${f.status}`)).toEqual([
      "a/1.jpg:uploaded",
      "a/2.jpg:waiting",
    ]);
  });

  it("counts duplicates as sent and groups by root", () => {
    const files = [
      file("案例/a.jpg", "uploaded", 10),
      file("案例/b.jpg", "skipped-duplicate", 0),
      file("案例 (2)/c.jpg", "uploading", 4),
    ];
    expect(overallProgress(files)).toEqual({ settled: 2, total: 3, sent: 24, bytes: 30 });
    const roots = rootProgress(files, ["案例", "案例 (2)"]);
    expect(roots.map((r) => [r.root, r.settled, r.total])).toEqual([
      ["案例", 2, 2],
      ["案例 (2)", 0, 1],
    ]);
  });

  it("does not count renames as skips", () => {
    const plan = {
      skipped: [
        { path: "a.psd", reason: "unsupported" },
        { path: "b.pdf", reason: "name-conflict-renamed" },
      ],
    } as never;
    expect(realSkips(plan).map((s) => s.path)).toEqual(["a.psd"]);
  });
});
