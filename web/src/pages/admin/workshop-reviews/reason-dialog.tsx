import { useState } from "react";
import { Alert, Button, Input, Select, Space } from "antd";
import { AppModal } from "@/components/ui/product/app-modal";

export function ReasonDialog({ title, presets = false, busy, error, onClose, onConfirm }: {
    title: string; presets?: boolean; busy: boolean; error?: string; onClose: () => void; onConfirm: (reason: string) => void;
}) {
    const [preset, setPreset] = useState<string>();
    const [text, setText] = useState("");
    const reason = [preset, text.trim()].filter(Boolean).join("；");
    return <AppModal flush open footer={null} closable={!busy} maskClosable={!busy} keyboard={!busy} onCancel={() => { if (!busy) onClose(); }}>
        <div className="space-y-4 p-6"><h2 className="text-lg font-medium">{title}</h2>
            {presets && <Select className="w-full" aria-label="常见原因" placeholder="常见原因" allowClear value={preset} disabled={busy} onChange={setPreset} options={["素材授权不明", "内容不完整", "与已有作品重复"].map((value) => ({ value, label: value }))} />}
            <label className="block space-y-2"><span>原因（必填）</span><Input.TextArea value={text} disabled={busy} onChange={(event) => setText(event.target.value)} rows={4} placeholder={presets ? "选择常见原因或填写具体说明" : "请填写下架原因，作者可以看到"} /></label>
            {error && <Alert type="error" title={error} />}
            <Space><Button type="primary" danger disabled={!reason} loading={busy} onClick={() => onConfirm(reason)}>确认</Button><Button disabled={busy} onClick={onClose}>取消</Button></Space>
        </div>
    </AppModal>;
}
