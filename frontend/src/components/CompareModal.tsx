import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import {
  clamp,
  formatBytes,
  formatDimensions,
  outputFormatLabel,
  savingsLabel,
  sourceFormatLabel,
} from "@/domain/format";
import type { ImageItem } from "@/domain/types";
import { CloseIcon } from "./icons";

interface CompareModalProps {
  item: ImageItem;
  onClose: () => void;
  onSave: (id: string) => void;
}

export function CompareModal({ item, onClose, onSave }: CompareModalProps) {
  const [position, setPosition] = useState(50);
  const [stageHeight, setStageHeight] = useState(460);
  const stageRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const result = item.result;

  useLayoutEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const ratio = item.info.width / Math.max(1, item.info.height);
    const width = stage.clientWidth || 800;
    setStageHeight(
      clamp(Math.round(width / Math.max(ratio, 0.2)), 220, Math.round(window.innerHeight * 0.56)),
    );
  }, [item.info.width, item.info.height]);

  const close = useCallback(() => {
    onClose();
  }, [onClose]);

  useEffect(() => {
    closeRef.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [close]);

  if (!result) return null;

  return (
    <div className="modal" id="compare-modal">
      <div className="modal-backdrop" data-close-compare onClick={close} />
      <div className="modal-card" role="dialog" aria-modal="true" aria-label="压缩前后对比">
        <header className="modal-head">
          <div>
            <h3>压缩前后对比</h3>
            <p id="compare-sub">
              {item.info.name} · {sourceFormatLabel(item.info.format)} →{" "}
              {outputFormatLabel(result.format)}
            </p>
          </div>
          <button
            ref={closeRef}
            className="icon-btn icon-btn--large"
            type="button"
            aria-label="关闭对比"
            onClick={close}
          >
            <CloseIcon />
          </button>
        </header>

        <div
          className="compare-stage"
          id="compare-stage"
          ref={stageRef}
          style={{ "--pos": `${position}%`, height: `${stageHeight}px` } as CSSProperties}
        >
          <img className="compare-img compare-img--after" src={result.preview} alt="压缩后" />
          <div className="compare-before">
            <img className="compare-img" src={item.info.preview} alt="原图" />
          </div>
          <div className="compare-divider">
            <span />
          </div>
          <span className="compare-tag compare-tag--before">原图</span>
          <span className="compare-tag compare-tag--after">压缩后</span>
          <input
            type="range"
            min={0}
            max={100}
            step={0.1}
            value={position}
            className="compare-scrub"
            aria-label="对比位置"
            onChange={(event) => setPosition(clamp(Number(event.target.value), 0, 100))}
          />
        </div>

        <footer className="compare-foot">
          <div className="compare-stat">
            <span>原图</span>
            <b>
              {formatDimensions(item.info.width, item.info.height)} · {formatBytes(item.info.bytes)}
            </b>
          </div>
          <div className="compare-stat">
            <span>压缩后</span>
            <b>
              {formatDimensions(result.width, result.height)} · {formatBytes(result.bytes)}
            </b>
          </div>
          <div className="compare-stat compare-stat--accent">
            <span>节省</span>
            <b style={{ color: result.savedPercent < 0 ? "var(--amber)" : undefined }}>
              {savingsLabel(result.savedPercent)} ·{" "}
              {formatBytes(Math.abs(item.info.bytes - result.bytes))}
            </b>
          </div>
          <button className="ghost-btn" type="button" onClick={() => onSave(item.info.id)}>
            保存这张
          </button>
        </footer>
      </div>
    </div>
  );
}
