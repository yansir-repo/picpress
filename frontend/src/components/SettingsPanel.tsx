import type { CSSProperties } from "react";
import {
  FORMAT_HINTS,
  FORMAT_OPTIONS,
  QUALITY_MAX,
  QUALITY_MIN,
  RESIZE_OPTIONS,
  isPng,
  qualityTag,
  type Settings,
} from "@/domain/settings";
import { ResizeMode } from "@/bindings/picpress/internal/imagepress/models";
import { clamp } from "@/domain/format";

interface SettingsPanelProps {
  settings: Settings;
  updateSettings: (patch: Partial<Settings>) => void;
  resetSettings: () => void;
}

type RangeStyle = CSSProperties & { "--pct": string };

function rangeStyle(value: number, min: number, max: number): RangeStyle {
  const percent = ((value - min) / (max - min)) * 100;
  return { "--pct": `${Math.min(100, Math.max(0, percent))}%` };
}

export function SettingsPanel({ settings, updateSettings, resetSettings }: SettingsPanelProps) {
  const png = isPng(settings.format);
  const tag = qualityTag(settings.quality, png);
  const longestPresets = [1280, 1920, 2560, 3840];

  return (
    <>
      <div className="sidebar-head">
        <div>
          <h2 className="sidebar-title">压缩设置</h2>
          <p className="sidebar-sub">调整后自动重新压缩</p>
        </div>
        <button
          className="reset-btn"
          id="reset-btn"
          type="button"
          title="恢复默认设置"
          onClick={resetSettings}
        >
          重置
        </button>
      </div>

      <div className="field">
        <div className="field-label">输出格式</div>
        <div className="segmented" id="format-seg" role="group" aria-label="输出格式">
          {FORMAT_OPTIONS.map((option) => (
            <button
              key={option.value}
              type="button"
              className={settings.format === option.value ? "is-active" : undefined}
              onClick={() => updateSettings({ format: option.value })}
            >
              {option.label}
            </button>
          ))}
        </div>
        <p className="field-hint" id="format-hint">
          {FORMAT_HINTS[settings.format] ?? ""}
        </p>
      </div>

      <div className={`field${png ? " is-disabled" : ""}`} id="quality-field">
        <div className="field-row">
          <div className="field-label">质量</div>
          <div className="quality-value">
            <span id="quality-number">{settings.quality}</span>
            <span className="quality-unit">%</span>
            <span className={`quality-tag ${tag.kind}`.trim()} id="quality-tag">
              {tag.label}
            </span>
          </div>
        </div>
        <input
          type="range"
          id="quality-range"
          className="range"
          min={QUALITY_MIN}
          max={QUALITY_MAX}
          step={1}
          value={settings.quality}
          disabled={png}
          aria-label="压缩质量"
          style={rangeStyle(settings.quality, QUALITY_MIN, QUALITY_MAX)}
          onChange={(event) => updateSettings({ quality: Number(event.target.value) })}
        />
        <div className="range-scale">
          <span>更小</span>
          <span>更好</span>
        </div>
        {png ? (
          <p className="field-hint" id="quality-hint">
            PNG 为无损格式，质量参数不生效
          </p>
        ) : null}
      </div>

      <div className="field">
        <div className="field-label">尺寸</div>
        <div
          className="segmented segmented--three"
          id="resize-seg"
          role="group"
          aria-label="尺寸调整方式"
        >
          {RESIZE_OPTIONS.map((option) => (
            <button
              key={option.value}
              type="button"
              className={settings.resizeMode === option.value ? "is-active" : undefined}
              onClick={() => updateSettings({ resizeMode: option.value })}
            >
              {option.label}
            </button>
          ))}
        </div>

        {settings.resizeMode === ResizeMode.ResizeLongEdge ? (
          <div className="resize-control" id="resize-longest">
            <div className="number-row">
              <input
                type="number"
                id="longest-input"
                className="number-input"
                min={32}
                max={20000}
                step={16}
                value={settings.maxLongEdge}
                aria-label="最大长边像素"
                onChange={(event) => {
                  const value = Number(event.target.value);
                  if (Number.isFinite(value) && value > 0) {
                    updateSettings({ maxLongEdge: Math.round(value) });
                  }
                }}
                onBlur={() =>
                  updateSettings({
                    maxLongEdge: clamp(Math.round(settings.maxLongEdge), 32, 20000),
                  })
                }
              />
              <span className="number-unit">px</span>
              <span className="number-note">短边等比缩放</span>
            </div>
            <div className="chips" id="longest-chips">
              {longestPresets.map((edge) => (
                <button
                  key={edge}
                  type="button"
                  className={settings.maxLongEdge === edge ? "is-active" : undefined}
                  onClick={() => updateSettings({ maxLongEdge: edge })}
                >
                  {edge}
                </button>
              ))}
            </div>
          </div>
        ) : null}

        {settings.resizeMode === ResizeMode.ResizePercent ? (
          <div className="resize-control" id="resize-percent">
            <div className="number-row">
              <input
                type="range"
                id="percent-range"
                className="range"
                min={10}
                max={100}
                step={1}
                value={settings.percent}
                aria-label="缩放百分比"
                style={rangeStyle(settings.percent, 10, 100)}
                onChange={(event) => updateSettings({ percent: Number(event.target.value) })}
              />
              <span className="percent-value" id="percent-value">
                {settings.percent}%
              </span>
            </div>
          </div>
        ) : null}
      </div>
    </>
  );
}
