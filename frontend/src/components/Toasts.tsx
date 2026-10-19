import { useAtomSet, useAtomValue } from "@effect/atom-react";
import { toastsAtom } from "@/state/atoms";

export function Toasts() {
  const toasts = useAtomValue(toastsAtom);
  const setToasts = useAtomSet(toastsAtom);

  return (
    <div className="toasts" id="toasts" aria-live="polite">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={`toast${toast.kind === "error" ? " is-error" : toast.kind === "warn" ? " is-warn" : ""}`}
        >
          <span className="toast-dot" />
          <span>{toast.message}</span>
          {toast.action ? (
            <button
              type="button"
              className="toast-action"
              onClick={() => {
                toast.action?.onClick();
                setToasts((previous) => previous.filter((entry) => entry.id !== toast.id));
              }}
            >
              {toast.action.label}
            </button>
          ) : null}
        </div>
      ))}
    </div>
  );
}
