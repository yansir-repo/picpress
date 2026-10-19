export function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export function bytesParts(bytes: number): [string, string] {
  if (!Number.isFinite(bytes) || bytes <= 0) return ["0", "B"];
  if (bytes < 1024) return [String(Math.round(bytes)), "B"];
  if (bytes < 1024 * 1024) {
    const value = bytes / 1024;
    return [trimNumber(value, value < 10 ? 1 : 0), "KB"];
  }
  const value = bytes / (1024 * 1024);
  return [trimNumber(value, value >= 100 ? 0 : value >= 10 ? 1 : 2), "MB"];
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "—";
  const [value, unit] = bytesParts(bytes);
  return `${value} ${unit}`;
}

function trimNumber(value: number, digits: number): string {
  const text = value.toFixed(digits);
  return text.includes(".") ? text.replace(/\.?0+$/, "") : text;
}

export function formatDimensions(width: number, height: number): string {
  if (!width || !height) return "—";
  return `${width} × ${height}`;
}

export function savingsLabel(percent: number): string {
  if (!Number.isFinite(percent)) return "—";
  const absolute = Math.abs(percent);
  const rounded = absolute >= 10 ? Math.round(absolute) : absolute.toFixed(1);
  return percent < 0 ? `+${rounded}%` : `−${rounded}%`;
}

export function sourceFormatLabel(format: string): string {
  const value = String(format || "").toLowerCase();
  const map: Record<string, string> = {
    jpeg: "JPG",
    jpg: "JPG",
    png: "PNG",
    webp: "WEBP",
    avif: "AVIF",
    heic: "HEIC",
    heif: "HEIF",
    gif: "GIF",
  };
  return map[value] || value.toUpperCase() || "IMG";
}

export function outputFormatLabel(format: string): string {
  const value = String(format || "").toLowerCase();
  const map: Record<string, string> = {
    jpeg: "JPG",
    png: "PNG",
    webp: "WEBP",
    avif: "AVIF",
  };
  return map[value] || value.toUpperCase();
}

export function baseName(path: string): string {
  if (!path) return "";
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] || path;
}

export function errorText(error: unknown): string {
  if (!error) return "未知错误";
  if (typeof error === "object" && error !== null) {
    const record = error as { message?: unknown; cause?: unknown };
    if (typeof record.message === "string" && record.message) {
      return record.message.replace(/^Error:\s*/, "");
    }
    if (record.cause) return errorText(record.cause);
  }
  return String(error).replace(/^Error:\s*/, "");
}
