import type { PushApi } from "@/hooks/usePush";

/** Confirmation card shown after a .picpress-config is imported (F1). */
export function ConfigConfirm({ push }: { push: PushApi }) {
  const pending = push.state?.pendingConfig;
  if (!pending) return null;
  const current = push.state?.connection;

  return (
    <div className="modal" role="dialog" aria-modal="true" aria-labelledby="config-title">
      <div className="modal-backdrop" onClick={push.dismissConfig} />
      <div className="modal-card push-card push-card--narrow">
        <div className="modal-head">
          <div>
            <h3 id="config-title">连接到「{pending.label}」？</h3>
            <p>上传地址：{pending.host}</p>
          </div>
        </div>
        {pending.notice ? <p className="push-notice">{pending.notice}</p> : null}
        <ul className="push-facts">
          <li>只有你拖进来并点了「开始上传」的文件才会上传。</li>
          <li>图片在本机压缩后再上传；视频只传抽帧，原片不会上传。</li>
          {current ? <li className="is-warn">会替换当前的连接「{current.label}」。</li> : null}
        </ul>
        <div className="push-actions">
          <button type="button" className="ghost-btn" onClick={push.dismissConfig}>
            取消
          </button>
          <button type="button" className="primary-btn push-primary" onClick={push.confirmConfig}>
            连接
          </button>
        </div>
      </div>
    </div>
  );
}
