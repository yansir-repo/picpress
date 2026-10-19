import { BrandMark } from "./icons";

export function Titlebar() {
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
        <span className="privacy-pill">
          <span className="privacy-dot" />
          全程本地处理
        </span>
      </div>
    </header>
  );
}
