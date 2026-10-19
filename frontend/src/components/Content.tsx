import { formatBytes } from "@/domain/format";
import type { SessionApi } from "@/hooks/useSession";
import { Dropzone } from "./Dropzone";
import { ItemCard } from "./ItemCard";
import { PlusIcon } from "./icons";

interface ContentProps {
  session: SessionApi;
}

export function Content({ session }: ContentProps) {
  const { items, compressing, progress, picking } = session;
  const hasItems = items.length > 0;
  const totalOriginal = items.reduce((sum, item) => sum + (item.info.bytes || 0), 0);

  let subtitle = "拖入图片，立即自动压缩";
  if (hasItems) {
    if (compressing) {
      subtitle =
        progress && progress.total > 0
          ? `正在压缩 ${progress.index} / ${progress.total}…`
          : "正在压缩…";
    } else {
      subtitle = `${items.length} 张图片 · 共 ${formatBytes(totalOriginal)}`;
    }
  }

  return (
    <section className="content" id="content">
      <div className="content-head">
        <div>
          <h1 className="content-title">图片</h1>
          <p className="content-sub" id="content-sub">
            {subtitle}
          </p>
        </div>
        {hasItems ? (
          <div className="content-actions">
            <button
              className="ghost-btn"
              id="add-more-btn"
              type="button"
              title="添加图片 (⌘O)"
              disabled={picking}
              onClick={() => {
                void session.pickFiles();
              }}
            >
              <PlusIcon />
              添加
            </button>
            <button
              className="ghost-btn ghost-btn--danger"
              id="clear-btn"
              type="button"
              title="清空列表 (⌘K)"
              onClick={() => {
                void session.clearAll();
              }}
            >
              清空
            </button>
          </div>
        ) : null}
      </div>

      {hasItems ? (
        <div className="list-wrap" id="list-wrap">
          <div className="list" id="list">
            {items.map((item) => (
              <ItemCard
                key={item.info.id}
                item={item}
                onCompare={session.openCompare}
                onSave={(id) => {
                  void session.saveOne(id);
                }}
                onRemove={(id) => {
                  void session.removeItem(id);
                }}
              />
            ))}
          </div>
          <button
            className="add-strip"
            id="add-strip"
            type="button"
            disabled={picking}
            onClick={() => {
              void session.pickFiles();
            }}
          >
            <PlusIcon />
            拖入更多图片，或点击添加
          </button>
        </div>
      ) : (
        <Dropzone
          onPick={() => {
            void session.pickFiles();
          }}
        />
      )}
    </section>
  );
}
