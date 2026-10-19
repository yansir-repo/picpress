import { Schema } from "effect";
import { Format, ResizeMode, type Options } from "@/bindings/picpress/internal/imagepress/models";

export interface Settings {
  format: Format;
  quality: number;
  resizeMode: ResizeMode;
  maxLongEdge: number;
  percent: number;
}

export const DEFAULT_SETTINGS: Settings = {
  format: Format.FormatAuto,
  quality: 82,
  resizeMode: ResizeMode.ResizeNone,
  maxLongEdge: 2560,
  percent: 100,
};

export const QUALITY_MIN = 10;
export const QUALITY_MAX = 100;

const STORAGE_KEY = "picpress.settings.v2";

const StoredSettings = Schema.Struct({
  format: Schema.Literals([
    Format.FormatAuto,
    Format.FormatJPEG,
    Format.FormatPNG,
    Format.FormatWebP,
    Format.FormatAVIF,
  ]),
  quality: Schema.Number,
  resizeMode: Schema.Literals([
    ResizeMode.ResizeNone,
    ResizeMode.ResizeLongEdge,
    ResizeMode.ResizePercent,
  ]),
  maxLongEdge: Schema.Number,
  percent: Schema.Number,
});

const decodeStored = Schema.decodeUnknownSync(StoredSettings);

export const FORMAT_OPTIONS: ReadonlyArray<{ value: Format; label: string }> = [
  { value: Format.FormatAuto, label: "智能" },
  { value: Format.FormatJPEG, label: "JPG" },
  { value: Format.FormatPNG, label: "PNG" },
  { value: Format.FormatWebP, label: "WebP" },
  { value: Format.FormatAVIF, label: "AVIF" },
];

export const RESIZE_OPTIONS: ReadonlyArray<{ value: ResizeMode; label: string }> = [
  { value: ResizeMode.ResizeNone, label: "原尺寸" },
  { value: ResizeMode.ResizeLongEdge, label: "限制长边" },
  { value: ResizeMode.ResizePercent, label: "按比例" },
];

export const FORMAT_HINTS: Partial<Record<Format, string>> = {
  [Format.FormatAuto]: "自动从 JPG / WebP / PNG 中选出体积最小的结果",
  [Format.FormatJPEG]: "兼容性最好，适合照片；透明区域会填充背景色",
  [Format.FormatPNG]: "无损格式，适合截图、图标和需要透明背景的图片",
  [Format.FormatWebP]: "现代格式，通常比同质量 JPG 小 25% 左右",
  [Format.FormatAVIF]: "压缩率最高，编码稍慢，较老的软件可能打不开",
};

export function loadSettings(): Settings {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...DEFAULT_SETTINGS };
    return normalizeSettings(decodeStored(JSON.parse(raw)));
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
}

export function saveSettings(settings: Settings): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch {
    // Storage can be unavailable; the app still works for this session.
  }
}

export function normalizeSettings(settings: Settings): Settings {
  return {
    format: isFormat(settings.format) ? settings.format : DEFAULT_SETTINGS.format,
    quality: Math.round(clampNumber(settings.quality, QUALITY_MIN, QUALITY_MAX, 82)),
    resizeMode: isResizeMode(settings.resizeMode)
      ? settings.resizeMode
      : DEFAULT_SETTINGS.resizeMode,
    maxLongEdge: Math.round(clampNumber(settings.maxLongEdge, 32, 20000, 2560)),
    percent: Math.round(clampNumber(settings.percent, 10, 100, 100)),
  };
}

export function toGoOptions(settings: Settings): Options {
  return {
    format: settings.format,
    quality: settings.quality,
    resizeMode: settings.resizeMode,
    maxLongEdge: settings.maxLongEdge,
    percent: settings.percent,
    background: "#ffffff",
  };
}

export function qualityTag(quality: number, isPng: boolean): { label: string; kind: string } {
  if (isPng) return { label: "无损", kind: "" };
  if (quality >= 90) return { label: "近乎无损", kind: "is-max" };
  if (quality >= 75) return { label: "推荐", kind: "" };
  if (quality >= 50) return { label: "更小", kind: "is-strong" };
  return { label: "极限", kind: "is-strong" };
}

export function isPng(format: Format): boolean {
  return format === Format.FormatPNG;
}

function isFormat(value: unknown): value is Format {
  return (
    value === Format.FormatAuto ||
    value === Format.FormatJPEG ||
    value === Format.FormatPNG ||
    value === Format.FormatWebP ||
    value === Format.FormatAVIF
  );
}

function isResizeMode(value: unknown): value is ResizeMode {
  return (
    value === ResizeMode.ResizeNone ||
    value === ResizeMode.ResizeLongEdge ||
    value === ResizeMode.ResizePercent
  );
}

function clampNumber(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, value));
}
