import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { CanvasNode } from "../src/components/canvas/canvas-node";
import { WorkshopNodeContent } from "../src/components/workshop/workshop-canvas-viewer";
import { CanvasNodeType, type CanvasNodeData } from "../src/types/canvas";
import { workshopFileUrl } from "../src/lib/workshop-media";

const noAction = () => undefined;
const node: CanvasNodeData = { id: "workshop-preview:node", title: "公开节点", type: CanvasNodeType.Text, width: 300, height: 200, position: { x: 0, y: 0 } };
const file = workshopFileUrl("a".repeat(64), "version");

test("workshop node rendering escapes HTML and leaves markdown as plain text", () => {
    const data = { ...node, metadata: { content: '<img src=x onerror="alert(1)">**文字**' } };
    const html = renderToStaticMarkup(<CanvasNode data={data} scale={1} readOnly isSelected={false} isRelated={false} isFocusRelated={false} isConnectionTarget={false} showImageInfo={false}
        renderReadOnlyContent={(item) => <WorkshopNodeContent node={item} allowDownload={false} />}
        onMouseDown={noAction} onHoverStart={noAction} onHoverEnd={noAction} onConnectStart={noAction} onResize={noAction} onContentChange={noAction} onContextMenu={noAction} />);
    expect(html).toContain("&lt;img");
    expect(html).toContain("**文字**");
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<strong>文字</strong>");
    expect(html).not.toContain("contenteditable");
});

test("public HTML, SVG, markdown and processing nodes use the same inert renderer", () => {
    for (const type of [CanvasNodeType.Html, CanvasNodeType.Svg, CanvasNodeType.Markdown, CanvasNodeType.Upscale]) {
        const html = renderToStaticMarkup(<CanvasNode data={{ ...node, type, metadata: { content: "<script>unsafe()</script>" } }} scale={1} readOnly isSelected={false} isRelated={false} isFocusRelated={false} isConnectionTarget={false} showImageInfo={false}
            renderReadOnlyContent={(item) => <WorkshopNodeContent node={item} allowDownload={false} />}
            onMouseDown={noAction} onHoverStart={noAction} onHoverEnd={noAction} onConnectStart={noAction} onResize={noAction} onContentChange={noAction} onContextMenu={noAction} />);
        expect(html).toContain("&lt;script&gt;");
        expect(html).not.toContain("<script>");
        expect(html).not.toContain("<iframe");
        expect(html).not.toContain("<input");
    }
});

test("media players use workshop URLs, preserve image proportions and respect the download setting", () => {
    for (const type of [CanvasNodeType.Image, CanvasNodeType.Drawing, CanvasNodeType.Video, CanvasNodeType.Audio]) {
        const html = renderToStaticMarkup(<WorkshopNodeContent node={{ ...node, type, metadata: { content: file } }} allowDownload={false} />);
        expect(html).toContain(`src="${file}"`);
        if (type === CanvasNodeType.Image || type === CanvasNodeType.Drawing) expect(html).toContain("object-contain");
        else { expect(html).toContain("controls"); expect(html).toContain('controlsList="nodownload"'); }
    }
    const managed = renderToStaticMarkup(<WorkshopNodeContent node={{ ...node, type: CanvasNodeType.Video, metadata: { content: file } }} allowDownload />);
    expect(managed).not.toContain("nodownload");
    const privateReference = renderToStaticMarkup(<WorkshopNodeContent node={{ ...node, type: CanvasNodeType.Image, metadata: { content: "/api/resources/private/file" } }} allowDownload={false} />);
    expect(privateReference).not.toContain("<img");
});
