import { useState } from "react";
import type { CustomS3 } from "@/bindings/picpress/internal/push/models";
import type { PushApi } from "@/hooks/usePush";

const EMPTY: CustomS3 = {
  label: "",
  endpoint: "",
  region: "auto",
  bucket: "",
  prefix: "",
  accessKeyId: "",
  secretAccessKey: "",
  sessionToken: "",
};

const FIELDS: Array<{
  key: keyof CustomS3;
  label: string;
  secret?: boolean;
  placeholder?: string;
}> = [
  { key: "endpoint", label: "Endpoint", placeholder: "https://<account>.r2.cloudflarestorage.com" },
  { key: "bucket", label: "Bucket" },
  { key: "prefix", label: "前缀", placeholder: "例如 inbox/" },
  { key: "region", label: "区域" },
  { key: "accessKeyId", label: "Access key" },
  { key: "secretAccessKey", label: "Secret", secret: true },
  { key: "sessionToken", label: "Session token（可选）", secret: true },
];

/** Connection section of the sidebar (F1) with the advanced S3 form (F3). */
export function PushSettings({ push }: { push: PushApi }) {
  const [advanced, setAdvanced] = useState(false);
  const [form, setForm] = useState<CustomS3>(EMPTY);
  const state = push.state;
  const connection = state?.connection;

  return (
    <div className="push-settings">
      <div className="field">
        <div className="field-label">推送上传</div>
        {connection ? (
          <div className="push-conn">
            <b>{connection.label}</b>
            <span>{connection.host}</span>
            <button type="button" className="reset-btn" onClick={push.removeConnection}>
              移除
            </button>
          </div>
        ) : (
          <p className="field-hint">没有连接。所有图片只在本机处理。</p>
        )}
        <button type="button" className="ghost-btn push-import" onClick={push.pickConfig}>
          导入配置…
        </button>
      </div>

      <button type="button" className="push-disclosure" onClick={() => setAdvanced(!advanced)}>
        {advanced ? "收起高级" : "高级"}
      </button>
      {advanced ? (
        <form
          className="push-form"
          onSubmit={(event) => {
            event.preventDefault();
            void push.saveCustomS3(form).then((ok) => ok && setForm(EMPTY));
          }}
        >
          <p className="field-hint">自定义 S3 兼容地址，开发联调用。密钥存入系统钥匙串。</p>
          {FIELDS.map((field) => (
            <label key={field.key}>
              <span>{field.label}</span>
              <input
                className="number-input"
                type={field.secret ? "password" : "text"}
                autoComplete="off"
                spellCheck={false}
                placeholder={field.placeholder}
                value={form[field.key]}
                onChange={(event) => setForm({ ...form, [field.key]: event.target.value })}
              />
            </label>
          ))}
          <button type="submit" className="ghost-btn">
            保存并连接
          </button>
        </form>
      ) : null}
      {state ? (
        <p className="push-about">
          设备 ID {state.deviceId} · 版本 {state.appVersion}
          {state.videoSupported ? "" : " · 未找到视频抽帧组件，视频会被跳过"}
        </p>
      ) : null}
    </div>
  );
}
