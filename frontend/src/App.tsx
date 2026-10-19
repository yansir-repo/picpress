import { useSession } from "@/hooks/useSession";
import { useRuntimeEvents } from "@/hooks/useRuntimeEvents";
import { CompareModal } from "@/components/CompareModal";
import { Content } from "@/components/Content";
import { Sidebar } from "@/components/Sidebar";
import { Titlebar } from "@/components/Titlebar";
import { Toasts } from "@/components/Toasts";

export function App() {
  const session = useSession();
  useRuntimeEvents(session);

  const compareItem = session.compareId
    ? session.items.find((item) => item.info.id === session.compareId)
    : undefined;

  return (
    <div className="app">
      <Titlebar />
      <main className="workspace" data-file-drop-target="">
        <Content session={session} />
        <Sidebar session={session} />
      </main>
      {compareItem ? (
        <CompareModal
          key={compareItem.info.id}
          item={compareItem}
          onClose={session.closeCompare}
          onSave={(id) => {
            void session.saveOne(id);
          }}
        />
      ) : null}
      <Toasts />
    </div>
  );
}
