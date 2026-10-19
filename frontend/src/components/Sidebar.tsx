import type { SessionApi } from "@/hooks/useSession";
import { SettingsPanel } from "./SettingsPanel";
import { SummaryPanel } from "./SummaryPanel";

interface SidebarProps {
  session: SessionApi;
}

export function Sidebar({ session }: SidebarProps) {
  return (
    <aside className="sidebar">
      <div className="sidebar-scroll">
        <SettingsPanel
          settings={session.settings}
          updateSettings={session.updateSettings}
          resetSettings={session.resetSettings}
        />
      </div>
      <SummaryPanel
        summary={session.summary}
        compressing={session.compressing}
        saving={session.saving}
        onSaveAll={() => {
          void session.saveAll();
        }}
      />
    </aside>
  );
}
