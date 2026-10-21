import { useState } from "react";
import type { Plan } from "@/bindings/picpress/internal/push/models";
import { formatBytes } from "@/domain/format";
import {
  REASON_LABELS,
  STATUS_LABELS,
  isActive,
  overallProgress,
  realSkips,
  renames,
  rootProgress,
} from "@/domain/push";
import { toGoOptions, type Settings } from "@/domain/settings";
import type { PushApi } from "@/hooks/usePush";

interface PushPanelProps {
  push: PushApi;
  settings: Settings;
  onLocalOnly: (paths: string[]) => void;
}

/** The prepare panel (F4) and, once started, the upload view (F11). */
export function PushPanel({ push, settings, onLocalOnly }: PushPanelProps) {
  const state = push.state;
  const plan = state?.plan;
  if (!state || !plan) return null;
  const started = state.phase !== "" || (state.files?.length ?? 0) > 0;

  return (
    <div className="modal" role="dialog" aria-modal="true" aria-labelledby="push-title">
      <div className="modal-backdrop" />
      <div className="modal-card push-card">
        {started ? (
          <UploadView push={push} plan={plan} />
        ) : (
          <PrepareView push={push} plan={plan} settings={settings} onLocalOnly={onLocalOnly} />
        )}
      </div>
    </div>
  );
}

function PrepareView({ push, plan, settings, onLocalOnly }: PushPanelProps & { plan: Plan }) {
  const [preset, setPreset] = useState<"current" | "text">("current");
  const [confirmed, setConfirmed] = useState(false);
  const [showSkips, setShowSkips] = useState(false);
  const skips = realSkips(plan);
  const renamed = renames(plan);
  const blocked = plan.blocked ?? [];
  const confirmReasons = plan.confirmReasons ?? [];
  const entries = plan.entries ?? [];
  const canStart =
    entries.length > 0 && blocked.length === 0 && (confirmReasons.length === 0 || confirmed);
  const images = entries.filter((entry) => entry.kind === "image").map((entry) => entry.path);

  return (
    <>
      <div className="modal-head">
        <div>
          <h3 id="push-title">准备上传</h3>
          <p>{(plan.roots ?? []).join("、")} · 确认之前不会上传任何文件</p>
        </div>
      </div>

      <div className="push-stats">
        <Stat label="图片" value={plan.images} note="本机压缩后上传" />
        <Stat label="文档" value={plan.documents} note="原样上传" />
        <Stat label="视频" value={plan.videos} note="只传抽帧，原片不会上传" />
        <Stat label="跳过" value={skips.length} note="见下方原因" />
      </div>
      <p className="push-total">原始大小共 {formatBytes(plan.totalBytes)}（视频原片不计入）</p>

      {blocked.map((reason) => (
        <p key={reason} className="push-alert is-error">
          {reason}
        </p>
      ))}
      {blocked.length === 0 && confirmReasons.length > 0 ? (
        <label className="push-alert is-warn push-confirm">
          <input
            type="checkbox"
            checked={confirmed}
            onChange={(event) => setConfirmed(event.target.checked)}
          />
          {confirmReasons.join("；")}，确认要上传这么多吗？
        </label>
      ) : null}

      {plan.images > 0 ? (
        <div className="field push-preset">
          <div className="field-label">图片压缩方式</div>
          <div className="segmented" role="group" aria-label="图片压缩方式">
            <button
              type="button"
              className={preset === "current" ? "is-active" : undefined}
              onClick={() => setPreset("current")}
            >
              当前设置
            </button>
            <button
              type="button"
              className={preset === "text" ? "is-active" : undefined}
              onClick={() => setPreset("text")}
            >
              文字图
            </button>
          </div>
          <p className="field-hint">
            {preset === "text"
              ? "证书、参数表等带小字的图：不缩小尺寸，高质量 WebP。"
              : "按右侧的压缩设置处理照片。"}
          </p>
        </div>
      ) : null}

      {skips.length + renamed.length > 0 ? (
        <div className="push-skips">
          <button
            type="button"
            className="push-disclosure"
            onClick={() => setShowSkips(!showSkips)}
          >
            {showSkips ? "收起" : "查看"}跳过 {skips.length} 个
            {renamed.length > 0 ? `、改名 ${renamed.length} 个` : ""}的文件
          </button>
          {showSkips ? (
            <ul className="push-list">
              {skips.map((skip) => (
                <li key={`s-${skip.path}`}>
                  <span className="push-path">{skip.path}</span>
                  <span className="push-reason">{skip.detail || REASON_LABELS[skip.reason]}</span>
                </li>
              ))}
              {renamed.map((skip) => (
                <li key={`r-${skip.path}`}>
                  <span className="push-path">{skip.path}</span>
                  <span className="push-reason">云端名称：{skip.renamedTo}</span>
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}

      <div className="push-actions">
        <button type="button" className="ghost-btn" onClick={push.closePlan}>
          取消
        </button>
        {images.length > 0 ? (
          <button
            type="button"
            className="ghost-btn"
            title="不上传，只在本机压缩图片"
            onClick={() => {
              onLocalOnly(images);
              push.closePlan();
            }}
          >
            只在本地压缩
          </button>
        ) : null}
        <button
          type="button"
          className="primary-btn push-primary"
          disabled={!canStart}
          onClick={() => push.start({ kind: preset, options: toGoOptions(settings) })}
        >
          开始上传 {entries.length} 个文件
        </button>
      </div>
    </>
  );
}

function Stat({ label, value, note }: { label: string; value: number; note: string }) {
  return (
    <div className="push-stat">
      <span>{label}</span>
      <b>{value}</b>
      <small>{note}</small>
    </div>
  );
}

function UploadView({ push, plan }: { push: PushApi; plan: Plan }) {
  const state = push.state!;
  const files = state.files ?? [];
  const total = overallProgress(files);
  const roots = rootProgress(files, plan.roots ?? []);
  const active = isActive(state.phase);
  const failed = files.filter((file) => file.status === "failed");
  const percent = total.bytes > 0 ? Math.round((total.sent / total.bytes) * 100) : 0;

  let title = "正在上传";
  if (state.phase === "waiting-network") title = "正在等待网络";
  else if (state.phase === "paused") title = "已暂停";
  else if (state.phase === "canceled") title = "已取消";
  else if (state.phase === "done") title = "上传完成";
  else if (state.phase === "fatal") title = "上传已停止";

  return (
    <>
      <div className="modal-head">
        <div>
          <h3 id="push-title">{title}</h3>
          <p>
            {total.settled} / {total.total} 个文件 · {formatBytes(total.sent)} /{" "}
            {formatBytes(total.bytes)}
          </p>
        </div>
      </div>

      {state.message ? (
        <p className={`push-alert ${state.phase === "fatal" ? "is-error" : "is-warn"}`}>
          {state.message}
        </p>
      ) : null}

      <div className="push-bar">
        <span style={{ width: `${percent}%` }} />
      </div>
      {roots.length > 1 ? (
        <ul className="push-roots">
          {roots.map((root) => (
            <li key={root.root}>
              <span className="push-path">{root.root}</span>
              <span>
                {root.settled} / {root.total}
              </span>
            </li>
          ))}
        </ul>
      ) : null}

      <ul className="push-list push-files">
        {files.map((file) => (
          <li key={file.source} className={`is-${file.status}`}>
            <span className="push-path">{file.source}</span>
            <span className="push-reason">
              {file.status === "uploading" && file.total > 0
                ? `${Math.round((file.sent / file.total) * 100)}%`
                : file.status === "failed"
                  ? file.detail || REASON_LABELS[file.reason ?? ""] || "失败"
                  : STATUS_LABELS[file.status]}
            </span>
          </li>
        ))}
      </ul>

      {state.summary ? <p className="push-summary">{state.summary.text}</p> : null}

      <div className="push-actions">
        {active ? (
          <>
            <button type="button" className="ghost-btn ghost-btn--danger" onClick={push.cancel}>
              取消
            </button>
            <button type="button" className="primary-btn push-primary" onClick={push.pause}>
              暂停
            </button>
          </>
        ) : (
          <>
            <button type="button" className="ghost-btn" onClick={push.closePlan}>
              关闭
            </button>
            {state.canResume ? (
              <button type="button" className="ghost-btn" onClick={push.cancel}>
                取消这一批
              </button>
            ) : null}
            {failed.length > 0 && !state.canResume ? (
              <button type="button" className="ghost-btn" onClick={push.retryFailed}>
                重试失败的 {failed.length} 个
              </button>
            ) : null}
            {state.canResume ? (
              <button type="button" className="primary-btn push-primary" onClick={push.resume}>
                继续
              </button>
            ) : state.summary ? (
              <button type="button" className="primary-btn push-primary" onClick={push.copySummary}>
                复制为文字
              </button>
            ) : null}
          </>
        )}
      </div>
    </>
  );
}
