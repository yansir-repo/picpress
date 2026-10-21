import * as Atom from "effect/reactivity/Atom";
import { loadSettings, type Settings } from "@/domain/settings";
import { summarize } from "@/domain/session";
import type { ImageItem, Progress, Toast } from "@/domain/types";
import type { PushSnapshot } from "@/bindings/picpress/models";

export const settingsAtom = Atom.make<Settings>(loadSettings()).pipe(Atom.keepAlive);

export const itemsAtom = Atom.make<ImageItem[]>([]).pipe(Atom.keepAlive);

export const compressingAtom = Atom.make(false).pipe(Atom.keepAlive);

export const pickingAtom = Atom.make(false).pipe(Atom.keepAlive);

export const savingAtom = Atom.make(false).pipe(Atom.keepAlive);

export const progressAtom = Atom.make<Progress | null>(null).pipe(Atom.keepAlive);

export const compareIdAtom = Atom.make<string | null>(null).pipe(Atom.keepAlive);

export const toastsAtom = Atom.make<Toast[]>([]).pipe(Atom.keepAlive);

export const summaryAtom = Atom.make((get) => summarize(get(itemsAtom))).pipe(Atom.keepAlive);

/** Mirror of the Go push state (null until the first snapshot arrives). */
export const pushAtom = Atom.make<PushSnapshot | null>(null).pipe(Atom.keepAlive);
