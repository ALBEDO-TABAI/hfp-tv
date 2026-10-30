import { create } from "zustand";
import type { PlatformMe, PlatformSession } from "@/services/api/platform";

type PlatformSessionState = {
    mode: "unknown" | "local" | "hosted";
    me: PlatformMe | null;
    setSession: (session: PlatformSession) => void;
    setMe: (me: PlatformMe | null) => void;
};

export const usePlatformSession = create<PlatformSessionState>()((set) => ({
    mode: "unknown",
    me: null,
    setSession: (session) => set({ mode: session.mode, me: session.mode === "hosted" ? session.me : null }),
    setMe: (me) => set({ me }),
}));

export const isHostedMode = () => usePlatformSession.getState().mode === "hosted";
export function hostedCapability(name: keyof PlatformMe["capabilities"]) {
    const { mode, me } = usePlatformSession.getState();
    return mode !== "hosted" || me?.capabilities[name] === true;
}
