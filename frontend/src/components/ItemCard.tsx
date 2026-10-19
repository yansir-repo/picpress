import {
  baseName,
  formatBytes,
  formatDimensions,
  outputFormatLabel,
  savingsLabel,
  sourceFormatLabel,
} from "@/domain/format";
import type { ImageItem } from "@/domain/types";
import { CloseIcon, CompareIcon, DownloadIcon } from "./icons";

interface ItemCardProps {
  item: ImageItem;
  onCompare: (id: string) => void;
  onSave: (id: string) => void;
  onRemove: (id: string) => void;
}

export function ItemCard({ item, onCompare, onSave, onRemove }: ItemCardProps) {
  const { info, result } = item;
  const available = Boolean(result) && !item.updating && !item.error;

  return (
    <article
      className={`item${item.error ? " has-error" : ""}${item.updating ? " is-updating" : ""}`}
    >
      <div className="item-thumb">
        <img src={info.preview} alt={info.name} />
      </div>

      <div className="item-info">
        <div className="item-name-row">
          <span className="item-name">{info.name || "未命名图片"}</span>
          <span className="format-chip">{sourceFormatLabel(info.format)}</span>
        </div>
        <div className="item-meta">
          {formatDimensions(info.width, info.height)} · {formatBytes(info.bytes)}
        </div>
        {item.error ? (
          <div className="item-status is-error">{item.error}</div>
        ) : item.updating ? (
          <div className="item-status">
            <span className="spinner" />
            正在压缩…
          </div>
        ) : info.warning ? (
          <div className="item-status">{info.warning}</div>
        ) : item.savedPath ? (
          <div className="item-status">已保存 · {baseName(item.savedPath)}</div>
        ) : null}
      </div>

      <div className="item-result">
        <div className="result-line">
          <span className="result-size">
            {available && result ? formatBytes(result.bytes) : item.updating ? "—" : "未压缩"}
          </span>
          {available && result ? (
            <span className="result-format">{outputFormatLabel(result.format)}</span>
          ) : null}
        </div>
        <div className="result-detail">
          {available && result
            ? formatDimensions(result.width, result.height)
            : item.updating
              ? ""
              : "调整设置后将自动重试"}
        </div>
      </div>

      <div className="item-savings" hidden={!available || !result}>
        {available && result ? (
          <span
            className={`savings-pill${result.savedPercent < 0 ? " is-bigger" : ""}`}
            title={result.warning || "相比原图的大小变化"}
          >
            {savingsLabel(result.savedPercent)}
          </span>
        ) : null}
      </div>

      <div className="item-actions">
        <button
          className="icon-btn"
          type="button"
          title="对比"
          aria-label="对比原图与压缩结果"
          disabled={!available}
          onClick={() => onCompare(info.id)}
        >
          <CompareIcon />
        </button>
        <button
          className="icon-btn"
          type="button"
          title="保存"
          aria-label="保存这张图片"
          disabled={!available}
          onClick={() => onSave(info.id)}
        >
          <DownloadIcon />
        </button>
        <button
          className="icon-btn icon-btn--muted"
          type="button"
          title="移除"
          aria-label="从列表移除"
          onClick={() => onRemove(info.id)}
        >
          <CloseIcon />
        </button>
      </div>
    </article>
  );
}
