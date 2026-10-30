import { expect, test } from "bun:test";
import { diffMarketFiles, skillWebsiteTarget } from "../src/lib/skill-market";

test("authorization paths open in this app; absolute URLs use the external path", () => {
    expect(skillWebsiteTarget("/skills/authorize?code=ABC-123")).toBe("app");
    expect(skillWebsiteTarget("https://team.example/skills/authorize?code=ABC-123")).toBe("external");
    expect(skillWebsiteTarget("http://localhost:43123/skills/authorize?code=ABC-123")).toBe("external");
    for (const url of ["//other.example", "/\\other.example", "javascript:alert(1)"]) {
        expect(() => skillWebsiteTarget(url)).toThrow();
    }
});

test("release comparison finds additions, edits and deletions by path and sha256", () => {
    const file = (path: string, sha256: string) => ({ path, sha256, size: 10, mimeType: "text/plain" });
    const previous = [file("SKILL.md", "old"), file("same.txt", "same"), file("removed.txt", "removed")];
    const current = [file("SKILL.md", "new"), file("same.txt", "same"), file("new.txt", "added")];
    expect(diffMarketFiles(current, previous).map(({ path, change }) => ({ path, change }))).toEqual([
        { path: "SKILL.md", change: "modified" }, { path: "same.txt", change: "unchanged" },
        { path: "new.txt", change: "added" }, { path: "removed.txt", change: "deleted" },
    ]);
    expect(diffMarketFiles(current, []).every(file => file.change === "added")).toBe(true);
    expect(diffMarketFiles([], previous).every(file => file.change === "deleted")).toBe(true);
    expect(current).toHaveLength(3);
    expect(previous).toHaveLength(3);
});
