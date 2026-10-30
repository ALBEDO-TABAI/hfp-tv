import { Alert, Button } from "antd";
import { useEffect, useState } from "react";
import { getMarketFile } from "@/services/api/skill-market";
import { ApiError } from "@/services/api/request";
import { platformErrorMessage } from "@/lib/platform-account";

export function MarketFilePreview({ skillId, releaseId }: { skillId: string; releaseId: string }) {
    const [content, setContent] = useState("");
    const [offset, setOffset] = useState(0);
    const [eof, setEOF] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<unknown>(null);
    const unavailable = error instanceof ApiError && error.reason === "market_not_configured";
    useEffect(() => { setContent(""); setOffset(0); setEOF(false); setError(null); }, [skillId, releaseId]);
    const read = async () => {
        setBusy(true); setError(null);
        try { const value = await getMarketFile(skillId, releaseId, "SKILL.md", offset); if (value.encoding !== "utf-8") throw new Error("SKILL.md 不是可读文本"); setContent(previous => previous + value.content); setOffset(value.nextOffset); setEOF(value.eof); }
        catch (e) { setError(e); }
        finally { setBusy(false); }
    };
    return <section><h3>使用说明</h3>{content ? <pre>{content}</pre> : null}{error ? <Alert type={unavailable ? "info" : "error"} title={unavailable ? platformErrorMessage(error) : error instanceof Error ? error.message : "技能正文暂时无法读取"} /> : null}{!eof ? <Button loading={busy} onClick={() => void read()}>{offset ? "继续读取说明" : "读取 SKILL.md"}</Button> : null}</section>;
}
