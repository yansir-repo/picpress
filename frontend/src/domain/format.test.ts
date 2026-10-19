import { describe, expect, it } from "vitest";
import { formatBytes, formatDimensions, savingsLabel, sourceFormatLabel } from "./format";

describe("formatBytes", () => {
  it("formats small and large values", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1024)).toBe("1 KB");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(1024 * 1024)).toBe("1 MB");
  });
});

describe("savingsLabel", () => {
  it("uses a minus sign for savings and plus for growth", () => {
    expect(savingsLabel(86.6)).toBe("−87%");
    expect(savingsLabel(-12)).toBe("+12%");
    expect(savingsLabel(3.25)).toBe("−3.3%");
  });
});

describe("labels", () => {
  it("normalises source formats", () => {
    expect(sourceFormatLabel("jpeg")).toBe("JPG");
    expect(sourceFormatLabel("heic")).toBe("HEIC");
    expect(sourceFormatLabel("png")).toBe("PNG");
  });

  it("formats dimensions", () => {
    expect(formatDimensions(1408, 768)).toBe("1408 × 768");
    expect(formatDimensions(0, 0)).toBe("—");
  });
});
