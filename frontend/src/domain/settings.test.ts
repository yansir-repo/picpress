import { describe, expect, it } from "vitest";
import { Format, ResizeMode } from "@/bindings/picpress/internal/imagepress/models";
import { DEFAULT_SETTINGS, normalizeSettings, qualityTag, toGoOptions } from "./settings";

describe("settings", () => {
  it("clamps persisted values into range", () => {
    const normalized = normalizeSettings({
      format: Format.FormatWebP,
      quality: 300,
      resizeMode: ResizeMode.ResizePercent,
      maxLongEdge: 1,
      percent: 0,
    });
    expect(normalized.quality).toBe(100);
    expect(normalized.maxLongEdge).toBe(32);
    expect(normalized.percent).toBe(10);
  });

  it("keeps defaults when the format is invalid", () => {
    const normalized = normalizeSettings({
      ...DEFAULT_SETTINGS,
      format: "bogus" as Format,
    });
    expect(normalized.format).toBe(Format.FormatAuto);
  });

  it("maps to Go options", () => {
    expect(toGoOptions(DEFAULT_SETTINGS)).toMatchObject({
      format: Format.FormatAuto,
      quality: 82,
      resizeMode: ResizeMode.ResizeNone,
      background: "#ffffff",
    });
  });

  it("labels quality buckets", () => {
    expect(qualityTag(95, false).label).toBe("近乎无损");
    expect(qualityTag(82, false).label).toBe("推荐");
    expect(qualityTag(82, true).label).toBe("无损");
  });
});
