import type { ReactNode } from "react";
import { bytesParts, formatBytes, savingsLabel } from "@/domain/format";
import { canSave } from "@/domain/session";
import type { Summary } from "@/domain/types";
import { DownloadIcon } from "./icons";

interface SummaryPanelProps {
  summary: Summary;
  compressing: boolean;
  saving: boolean;
  onSaveAll: () => void;
  pushConnected: boolean;
}

export function SummaryPanel({
  summary,
  compressing,
  saving,
  onSaveAll,
  pushConnected,
}: SummaryPanelProps) {
  const hasItems = summary.count > 0;
  const hasResults = summary.resultCount > 0;
  const [value, unit] = bytesParts(Math.abs(summary.saved));
  const barWidth = Math.min(100, Math.abs(summary.percent));
  const saveEnabled = canSave(summary, compressing, saving);

  let metric: ReactNode = "—";
  let detail = "添加图片后显示压缩效果";
  if (hasItems && !hasResults) {
    metric = compressing ? "压缩中…" : "—";
    detail = compressing ? "正在计算压缩效果" : "等待压缩结果";
  } else if (hasResults) {
    metric = (
      <>
        <span>{summary.bigger ? "+" : "−"}</span>
        <span>{value}</span>
        <span className="metric-unit">{unit}</span>
      </>
    );
    detail =
      summary.resultCount === summary.count
        ? `${formatBytes(summary.original)} → ${formatBytes(summary.output)}`
        : `${formatBytes(summary.original)} → ${formatBytes(summary.output)} · ${summary.resultCount}/${summary.count} 张`;
  }

  return (
    <div className="summary" id="summary">
      <div className="summary-top">
        <span className="summary-label">总体积变化</span>
        <span
          className={`summary-badge${summary.bigger ? " is-bigger" : ""}`}
          id="summary-badge"
          hidden={!hasResults}
        >
          {hasResults ? savingsLabel(summary.percent) : ""}
        </span>
      </div>
      <div className="summary-main">
        <div className="summary-metric" id="summary-metric">
          {metric}
        </div>
        <div className="summary-detail" id="summary-detail">
          {detail}
        </div>
      </div>
      <div className="summary-bar">
        <span
          id="summary-bar-fill"
          className={summary.bigger ? "is-bigger" : undefined}
          style={{ width: hasResults ? `${barWidth}%` : "0%" }}
        />
      </div>
      <div className="summary-rows" id="summary-rows" hidden={!hasResults}>
        <div>
          <span>原图</span>
          <b id="sum-original">{hasResults ? formatBytes(summary.original) : "—"}</b>
        </div>
        <div>
          <span>压缩后</span>
          <b id="sum-output">{hasResults ? formatBytes(summary.output) : "—"}</b>
        </div>
      </div>
      <button
        className="primary-btn"
        id="save-all-btn"
        type="button"
        disabled={!saveEnabled}
        onClick={onSaveAll}
      >
        <DownloadIcon />
        {saving ? "正在保存…" : compressing ? "压缩中…" : "保存全部"}
      </button>
      <p className="summary-foot">
        {pushConnected
          ? "这里的图片只在本机处理；只有在「准备上传」里确认的文件才会上传"
          : "图片只在本机处理，不会上传到任何服务器"}
      </p>
    </div>
  );
}
