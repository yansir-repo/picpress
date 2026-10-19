import { describe, expect, it } from "vitest";
import type { CompressResult, ImageItem, ItemInfo } from "./types";
import { applyResults, markUpdating, mergeInfos, summarize } from "./session";

function info(id: string, bytes = 1000): ItemInfo {
  return {
    id,
    name: `${id}.jpg`,
    path: `/tmp/${id}.jpg`,
    format: "jpeg",
    width: 100,
    height: 80,
    bytes,
    preview: "",
    hasAlpha: false,
    warning: "",
  };
}

function item(id: string, bytes = 1000): ImageItem {
  return { info: info(id, bytes), result: null, updating: false, error: "", savedPath: "" };
}

function result(id: string, bytes: number): CompressResult {
  return {
    id,
    format: "webp",
    width: 100,
    height: 80,
    bytes,
    originalBytes: 1000,
    originalWidth: 100,
    originalHeight: 80,
    savedPercent: 80,
    durationMs: 10,
    preview: "",
    outputPath: `/tmp/${id}.webp`,
    warning: "",
    error: "",
  };
}

describe("session helpers", () => {
  it("merges new info and updates existing info without duplicating", () => {
    const merged = mergeInfos([item("a")], [info("a", 1200), info("b")]);
    expect(merged).toHaveLength(2);
    expect(merged[0]?.info.bytes).toBe(1200);
    expect(merged[1]?.info.id).toBe("b");
  });

  it("marks every item as updating", () => {
    const updating = markUpdating([item("a"), item("b")]);
    expect(updating.every((entry) => entry.updating)).toBe(true);
  });

  it("applies results and errors by id", () => {
    const results = [result("a", 200), { ...result("b", 0), error: "boom" }];
    const next = applyResults([item("a"), item("b"), item("c")], results);
    expect(next[0]?.result?.bytes).toBe(200);
    expect(next[1]?.error).toBe("boom");
    expect(next[2]?.result).toBeNull();
  });

  it("summarizes only finished results", () => {
    const items = [
      { ...item("a", 1000), result: result("a", 200) },
      { ...item("b", 500), result: null },
    ];
    const summary = summarize(items);
    expect(summary.original).toBe(1000);
    expect(summary.output).toBe(200);
    expect(summary.percent).toBe(80);
    expect(summary.resultCount).toBe(1);
  });
});
