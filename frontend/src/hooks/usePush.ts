import { useAtomSet, useAtomValue } from "@effect/atom-react";
import { Events } from "@wailsio/runtime";
import * as Effect from "effect/Effect";
import { useCallback, useEffect } from "react";
import type { CustomS3, FileUpdate, Summary } from "@/bindings/picpress/internal/push/models";
import type { PushPreset, PushSnapshot } from "@/bindings/picpress/models";
import { errorText } from "@/domain/format";
import { mergeFiles } from "@/domain/push";
import * as push from "@/services/push";
import { pushAtom } from "@/state/atoms";
import { useToast } from "./useToast";

export interface PushApi {
  state: PushSnapshot | null;
  pickConfig: () => void;
  confirmConfig: () => void;
  dismissConfig: () => void;
  removeConnection: () => void;
  saveCustomS3: (input: CustomS3) => Promise<boolean>;
  start: (preset: PushPreset) => void;
  closePlan: () => void;
  pause: () => void;
  resume: () => void;
  cancel: () => void;
  retryFailed: () => void;
  copySummary: () => void;
}

/** Push state lives in Go; this hook mirrors it and forwards actions. */
export function usePush(): PushApi {
  const state = useAtomValue(pushAtom);
  const setState = useAtomSet(pushAtom);
  const toast = useToast();

  const refresh = useCallback(() => {
    void Effect.runPromise(push.state())
      .then(setState)
      .catch(() => {});
  }, [setState]);

  useEffect(() => {
    refresh();
    const off = [
      Events.On("push:state", (event) => setState(event.data as PushSnapshot)),
      Events.On("push:files", (event) =>
        setState((previous) =>
          previous ? mergeFiles(previous, event.data as FileUpdate[]) : previous,
        ),
      ),
      Events.On("push:phase", (event) => {
        const { phase, message } = event.data as { phase: string; message: string };
        setState((previous) => (previous ? { ...previous, phase, message } : previous));
      }),
      Events.On("push:error", (event) => toast(String(event.data), "error")),
      Events.On("push:done", (event) => toast((event.data as Summary).text, "success")),
    ];
    return () => {
      for (const unsubscribe of off) unsubscribe();
    };
  }, [refresh, setState, toast]);

  const run = useCallback(
    <A>(operation: Effect.Effect<A, Error>, onOk?: (value: A) => void) => {
      void Effect.runPromise(operation)
        .then((value) => onOk?.(value))
        .catch((error: unknown) => toast(errorText(error), "error"));
    },
    [toast],
  );

  return {
    state,
    pickConfig: () => run(push.pickConfig()),
    confirmConfig: () => run(push.confirmConfig(), () => toast("已连接", "success")),
    dismissConfig: () => run(push.dismissConfig()),
    removeConnection: () => run(push.removeConnection()),
    saveCustomS3: async (input) => {
      try {
        await Effect.runPromise(push.saveCustomS3(input));
        toast("已连接自定义 S3 地址", "success");
        return true;
      } catch (error) {
        toast(errorText(error), "error");
        return false;
      }
    },
    start: (preset) => run(push.start(preset)),
    closePlan: () => run(push.closePlan()),
    pause: () => run(push.pause()),
    resume: () => run(push.resume()),
    cancel: () => run(push.cancel()),
    retryFailed: () => run(push.retryFailed()),
    copySummary: () =>
      run(push.copySummary(), (ok) =>
        toast(ok ? "已复制，可以直接粘贴到聊天里" : "复制失败", ok ? "success" : "error"),
      ),
  };
}
