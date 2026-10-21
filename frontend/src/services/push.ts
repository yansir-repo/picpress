import * as Effect from "effect/Effect";
import { PushService } from "@/bindings/picpress";
import type { CustomS3 } from "@/bindings/picpress/internal/push/models";
import type { PushPreset, PushSnapshot } from "@/bindings/picpress/models";
import { WailsError } from "./wails";

function call<A>(operation: string, run: () => Promise<A>): Effect.Effect<A, WailsError> {
  return Effect.tryPromise({
    try: run,
    catch: (cause) => new WailsError(operation, cause),
  });
}

export const state = () => call<PushSnapshot>("读取上传状态", () => PushService.State());
export const pickConfig = () => call("导入配置", () => PushService.PickConfig());
export const confirmConfig = () => call("连接", () => PushService.ConfirmConfig());
export const dismissConfig = () => call("取消导入", () => PushService.DismissConfig());
export const removeConnection = () => call("移除连接", () => PushService.RemoveConnection());
export const saveCustomS3 = (input: CustomS3) =>
  call("保存 S3 地址", () => PushService.SaveCustomS3(input));
export const start = (preset: PushPreset) => call("开始上传", () => PushService.Start(preset));
export const closePlan = () => call("关闭", () => PushService.ClosePlan());
export const pause = () => call("暂停", () => PushService.Pause());
export const resume = () => call("继续", () => PushService.Resume());
export const cancel = () => call("取消上传", () => PushService.Cancel());
export const retryFailed = () => call("重试", () => PushService.RetryFailed());
export const copySummary = () => call("复制", () => PushService.CopySummary());
