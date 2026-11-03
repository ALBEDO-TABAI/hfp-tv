import { describe, expect, test } from "bun:test";

import { canvasGenerationTaskNodes } from "@/pages/canvas/canvas-generation-failure";
import type { GenerationTask } from "@/services/api/task-center";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";

const running = (updatedAt: string, progress = 0) =>
    ({ id: "task-1", type: "canvas_video", status: "running", stage: "上游生成中", progress, createdAt: "2026-11-03T09:52:55Z", startedAt: "2026-11-03T09:52:56Z", updatedAt, failureDiagnostics: { source: "unknown" } }) as unknown as GenerationTask;

describe("running task updates on the canvas", () => {
    test("a lease heartbeat that only moves updatedAt leaves the canvas untouched", () => {
        const node = { id: "video", type: CanvasNodeType.Video, x: 0, y: 0, width: 400, height: 225, metadata: {} } as CanvasNodeData;
        const initial = [node];
        const first = canvasGenerationTaskNodes(initial, "video", running("2026-11-03T09:53:00Z"));
        expect(first).not.toBe(initial);
        expect(first[0]!.metadata?.taskStatus).toBe("running");
        // Heartbeats renew the lease and bump updatedAt every few seconds.
        const heartbeat = canvasGenerationTaskNodes(first, "video", running("2026-11-03T09:53:04Z"));
        expect(heartbeat).toBe(first);
        // Real progress still reaches the node.
        const progressed = canvasGenerationTaskNodes(first, "video", running("2026-11-03T09:53:30Z", 50));
        expect(progressed).not.toBe(first);
        expect(progressed[0]!.metadata?.taskProgress).toBe(50);
    });
});
