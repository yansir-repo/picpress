import type { PushApi } from "@/hooks/usePush";
import type { SessionApi } from "@/hooks/useSession";
import { PushSettings } from "./PushSettings";
import { SettingsPanel } from "./SettingsPanel";
import { SummaryPanel } from "./SummaryPanel";

interface SidebarProps {
  session: SessionApi;
  push: PushApi;
}

export function Sidebar({ session, push }: SidebarProps) {
  return (
    <aside className="sidebar">
      <div className="sidebar-scroll">
        <SettingsPanel
          settings={session.settings}
          updateSettings={session.updateSettings}
          resetSettings={session.resetSettings}
        />
        <PushSettings push={push} />
      </div>
      <SummaryPanel
        summary={session.summary}
        pushConnected={Boolean(push.state?.connection)}
        compressing={session.compressing}
        saving={session.saving}
        onSaveAll={() => {
          void session.saveAll();
        }}
      />
    </aside>
  );
}
