import type { PushApi } from "@/hooks/usePush";
import { BrandMark } from "./icons";

export function Titlebar({ push }: { push: PushApi }) {
  const state = push.state;
  const connection = state?.connection;

  return (
    <header className="titlebar" id="titlebar">
      <div className="brand">
        <span className="brand-mark" aria-hidden="true">
          <BrandMark />
        </span>
        <span className="brand-name">PicPress</span>
        <span className="brand-tag">图片压缩</span>
      </div>
      <div className="titlebar-right">
        {state?.canResume && !state.plan ? (
          <button type="button" className="privacy-pill push-pill-btn" onClick={push.resume}>
            有未完成的上传 · 继续
          </button>
        ) : null}
        {connection ? (
          <span className="privacy-pill" title={`上传地址：${connection.host}`}>
            <span className="privacy-dot push-dot" />
            已连接：{connection.label}
          </span>
        ) : (
          <span className="privacy-pill">
            <span className="privacy-dot" />
            全程本地处理
          </span>
        )}
      </div>
    </header>
  );
}
