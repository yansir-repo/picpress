import { useAtomSet } from "@effect/atom-react";
import { useCallback } from "react";
import { toastsAtom } from "@/state/atoms";
import type { ToastAction, ToastKind } from "@/domain/types";

let nextToastId = 0;

export function useToast() {
  const setToasts = useAtomSet(toastsAtom);

  return useCallback(
    (message: string, kind: ToastKind = "info", action?: ToastAction) => {
      const id = ++nextToastId;
      setToasts((previous) => [...previous.slice(-2), { id, message, kind, action }]);
      window.setTimeout(
        () => setToasts((previous) => previous.filter((toast) => toast.id !== id)),
        action ? 6500 : 4200,
      );
    },
    [setToasts],
  );
}
