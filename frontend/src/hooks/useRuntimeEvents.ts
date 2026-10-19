import { useAtomSet } from "@effect/atom-react";
import { Events } from "@wailsio/runtime";
import { useEffect } from "react";
import type { Progress } from "@/domain/types";
import { progressAtom } from "@/state/atoms";
import type { SessionApi } from "./useSession";

export function useRuntimeEvents(session: SessionApi): void {
  const setProgress = useAtomSet(progressAtom);
  const { addPaths, clearAll, pickFiles, saveAll } = session;

  useEffect(() => {
    const unsubscribe = [
      Events.On("files-dropped", (event) => {
        const files = event.data;
        if (Array.isArray(files)) {
          void addPaths(files.filter((file): file is string => typeof file === "string"));
        }
      }),
      Events.On("compress:progress", (event) => {
        setProgress((event.data as Progress | null) ?? null);
      }),
      Events.On("menu:open", () => {
        void pickFiles();
      }),
      Events.On("menu:save-all", () => {
        void saveAll();
      }),
      Events.On("menu:clear", () => {
        void clearAll();
      }),
    ];

    return () => {
      for (const off of unsubscribe) off();
    };
  }, [addPaths, clearAll, pickFiles, saveAll, setProgress]);
}
