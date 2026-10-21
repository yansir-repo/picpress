import { usePush } from "@/hooks/usePush";
import { useSession } from "@/hooks/useSession";
import { useRuntimeEvents } from "@/hooks/useRuntimeEvents";
import { CompareModal } from "@/components/CompareModal";
import { ConfigConfirm } from "@/components/ConfigConfirm";
import { PushPanel } from "@/components/PushPanel";
import { Content } from "@/components/Content";
import { Sidebar } from "@/components/Sidebar";
import { Titlebar } from "@/components/Titlebar";
import { Toasts } from "@/components/Toasts";

export function App() {
  const session = useSession();
  const push = usePush();
  useRuntimeEvents(session);

  const compareItem = session.compareId
    ? session.items.find((item) => item.info.id === session.compareId)
    : undefined;

  return (
    <div className="app">
      <Titlebar push={push} />
      <main className="workspace" data-file-drop-target="">
        <Content session={session} />
        <Sidebar session={session} push={push} />
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
      <PushPanel
        push={push}
        settings={session.settings}
        onLocalOnly={(paths) => {
          void session.addPaths(paths);
        }}
      />
      <ConfigConfirm push={push} />
      <Toasts />
    </div>
  );
}
