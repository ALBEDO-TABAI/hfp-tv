import { Button } from "antd";
import { useRef, useState } from "react";
import { AppModal } from "@/components/ui/product/app-modal";
import { assertUserScope, type CapturedUserScope } from "@/lib/user-scope-guard";

// Capture scope at the entry point, before a lazy dialog can finish loading.
export function CanvasDeleteConfirmation({ title, description, scope, open = true, onConfirm, onCancel, afterClose }: {
    title: string;
    description: string;
    scope: CapturedUserScope;
    open?: boolean;
    onConfirm: (assertCurrent: () => void) => Promise<void>;
    onCancel: () => void;
    /** 离场动画结束后调用，父组件这时才卸载。 */
    afterClose?: () => void;
}) {
    const busy = useRef(false);
    const [deleting, setDeleting] = useState(false);
    const [error, setError] = useState("");
    const confirm = async () => {
        // 离场中的弹窗已经 inert；这里再挡一次，关闭之后不会再执行删除。
        if (busy.current || !open) return;
        busy.current = true;
        setDeleting(true);
        setError("");
        try {
            const assertCurrent = () => assertUserScope(scope);
            assertCurrent();
            await onConfirm(assertCurrent);
            assertCurrent();
            onCancel();
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : "删除失败，请稍后重试");
        } finally {
            busy.current = false;
            setDeleting(false);
        }
    };
    return <AppModal open={open} afterClose={afterClose} centered title={title} closable={!deleting} keyboard={!deleting} maskClosable={!deleting}
        onCancel={() => { if (!busy.current) onCancel(); }}
        footer={<><Button disabled={deleting} onClick={onCancel}>取消</Button><Button danger type="primary" loading={deleting} onClick={() => void confirm()}>删除</Button></>}>
        <p className="text-sm text-foreground/60">{description}</p>
        {error ? <p role="alert" className="mt-3 text-sm">{error}</p> : null}
    </AppModal>;
}
