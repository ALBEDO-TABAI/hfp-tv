import { create } from "zustand";
import type { PlatformIdentity } from "@/services/api/platform";

type PlatformIdentityStore = {
    identity: PlatformIdentity | null;
    setIdentity: (identity: PlatformIdentity | null) => void;
};

export const usePlatformIdentityStore = create<PlatformIdentityStore>()((set) => ({
    identity: null,
    setIdentity: (identity) => set({ identity }),
}));
