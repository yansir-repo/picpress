import * as Effect from "effect/Effect";
import { AppService } from "@/bindings/picpress";
import type {
  AddFilesResult,
  CompressResult,
  Options,
} from "@/bindings/picpress/internal/imagepress/models";
import type { SaveAllResult } from "@/bindings/picpress/models";

/** A failed Wails call, kept as a typed Effect error. */
export class WailsError extends Error {
  readonly _tag = "WailsError";

  constructor(
    readonly operation: string,
    override readonly cause: unknown,
  ) {
    const detail = cause instanceof Error ? cause.message : cause ? String(cause) : "";
    super(`${operation}失败${detail ? `：${detail}` : ""}`);
    this.name = "WailsError";
  }
}

function call<A>(operation: string, run: () => Promise<A>): Effect.Effect<A, WailsError> {
  return Effect.tryPromise({
    try: run,
    catch: (cause) => new WailsError(operation, cause),
  });
}

export const addFiles = (paths: string[]) =>
  call<AddFilesResult>("添加图片", () => AppService.AddFiles(paths));

export const pickFiles = () => call<AddFilesResult>("选择图片", () => AppService.PickFiles());

export const compress = (options: Options) =>
  call<CompressResult[] | null>("压缩图片", () => AppService.Compress([], options));

export const saveFile = (id: string) => call<string>("保存图片", () => AppService.SaveFile(id));

export const saveAll = (ids: string[] = []) =>
  call<SaveAllResult>("保存全部", () => AppService.SaveAll(ids));

export const remove = (id: string) => call<boolean>("移除图片", () => AppService.Remove(id));

export const clear = () => call<boolean>("清空图片", () => AppService.Clear());

export const revealPath = (path: string) =>
  call<boolean>("打开 Finder", () => AppService.RevealPath(path));
