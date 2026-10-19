import { DownloadIcon, DropIllustration } from "./icons";

interface DropzoneProps {
  onPick: () => void;
}

export function Dropzone({ onPick }: DropzoneProps) {
  return (
    <div
      className="dropzone"
      id="dropzone"
      role="button"
      aria-label="拖入或选择图片"
      onClick={(event) => {
        if (event.target instanceof HTMLElement && event.target.closest("button")) return;
        onPick();
      }}
    >
      <div className="dropzone-inner">
        <div className="drop-art" aria-hidden="true">
          <DropIllustration />
          <span className="drop-art-badge">
            <DownloadIcon />
          </span>
        </div>
        <h2 className="drop-title">把图片拖到这里</h2>
        <p className="drop-sub">松手即自动压缩，支持一次拖入多张</p>
        <button
          className="drop-btn"
          id="pick-btn"
          type="button"
          onClick={(event) => {
            event.stopPropagation();
            onPick();
          }}
        >
          选择图片
        </button>
        <p className="drop-formats">JPG · PNG · WebP · AVIF · HEIC · GIF</p>
      </div>
    </div>
  );
}
