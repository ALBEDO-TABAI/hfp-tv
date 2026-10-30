import type { ReactNode } from "react";
import { BrandLogoFrame, HfpWordmark } from "@/components/brand/brand-logo";

export function PlatformAuthCard({ title, children }: { title: string; children: ReactNode }) {
    return <main className="flex min-h-dvh items-center justify-center p-6" style={{ background: "var(--stage)", color: "var(--ink)" }}>
        <section className="w-full max-w-md p-8" style={{ background: "var(--stage-2)", border: "1px solid var(--hair)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-2)" }}>
            <div className="mb-6 flex items-center justify-center gap-3">
                <BrandLogoFrame className="grid size-10 place-items-center rounded-[var(--r-sm)]" logoClassName="size-8 object-contain" fallback={<span>h</span>} />
                <HfpWordmark />
            </div>
            <h1 className="mb-6 text-xl font-medium">{title}</h1>
            {children}
        </section>
    </main>;
}
