import { localForageStorage } from "@/lib/localforage-storage";
import { normalizeLocalAsset, normalizeLocalCanvasProject } from "@/lib/local-workspace-migration";
import { scopedLocalStorage, setActiveUserScope } from "@/lib/user-scope";
import { CANVAS_HISTORY_STORE_KEY, useCanvasHistoryStore } from "@/stores/canvas/use-canvas-history-store";
import { CANVAS_STORE_KEY, flushCanvasStorePersistence, useCanvasStore } from "@/stores/canvas/use-canvas-store";
import { captureUserScope } from "@/lib/user-scope-guard";
import { ASSET_STORE_KEY, flushAssetStorePersistence, useAssetStore } from "@/stores/use-asset-store";
import { preserveLegacyCacheOnlyAssetDrafts, usesWorkspaceAssetLibraryApi } from "@/services/workspace-asset-read";
import { CONFIG_STORE_KEY, defaultConfig, isSystemProxyBaseUrl, normalizeConfigSnapshot, useConfigStore, type AiConfig } from "@/stores/use-config-store";
import { CREATION_PREFERENCES_STORE_KEY, useCreationPreferencesStore } from "@/stores/use-creation-preferences-store";
import { PLUGIN_STORE_KEY, usePluginStore } from "@/stores/use-plugin-store";
import { defaultFeatureAvailability, useUserStore } from "@/stores/use-user-store";
import type { WorkspaceBootstrapPayload } from "@/services/api/workspace";
import { setWorkspaceCapabilitySnapshot } from "@/services/workspace-mode";
import { withGenerationConsumersPaused } from "@/services/generation-consumer-lifecycle";
import { isTeamDeployment } from "@/lib/deployment-mode";
import { teamWorkspaceScope } from "@/lib/team-auth";
import type { PlatformIdentity } from "@/services/api/platform";
import { teamSessionSignal } from "@/services/team-session";

/**
 * Select the persistence namespace before restoring workspace stores.
 * Team identity comes from the platform; the kernel still runs a local workspace.
 */
export async function applyUserSession(payload: WorkspaceBootstrapPayload, identity?: PlatformIdentity) {
    // Drain old result writers before changing the persistence namespace or stores.
    return withGenerationConsumersPaused(() => hydrateLocalWorkspace(payload, identity));
}

async function hydrateLocalWorkspace(payload: WorkspaceBootstrapPayload, identity?: PlatformIdentity) {
    const team = isTeamDeployment();
    if (team && !identity) throw new Error("请先登录");
    const scope = team && identity ? teamWorkspaceScope(identity.user.id, identity.workspace_id) : payload.workspace.id;
    const signal = teamSessionSignal();
    signal?.throwIfAborted();
    useUserStore.getState().setHydrated(false);
    setWorkspaceCapabilitySnapshot({
        contractVersion: payload.contractVersion,
        profile: "local",
        capabilities: payload.capabilities,
    });

    try {
        await Promise.all([flushCanvasStorePersistence(), flushAssetStorePersistence()]);
        signal?.throwIfAborted();
        setActiveUserScope(scope);

        const [persistedCanvas, persistedCanvasHistory, persistedAssets, persistedPlugins] = await Promise.all([
            localForageStorage.getItem(CANVAS_STORE_KEY),
            localForageStorage.getItem(CANVAS_HISTORY_STORE_KEY),
            localForageStorage.getItem(ASSET_STORE_KEY),
            localForageStorage.getItem(PLUGIN_STORE_KEY),
        ]);
        const persistedConfig = scopedLocalStorage.getItem(CONFIG_STORE_KEY);
        const persistedCreationPreferences = scopedLocalStorage.getItem(CREATION_PREFERENCES_STORE_KEY);

        signal?.throwIfAborted();
        usePluginStore.setState({ hydrated: false, runtimeStatuses: {}, pluginStates: {} });
        useUserStore.getState().setUser(payload.user);
        useUserStore.getState().setStorageMode("local");
        useUserStore.getState().setRuntimeLimits(payload.runtimeLimits);
        useUserStore.getState().setFeatures({ ...defaultFeatureAvailability, ...payload.features });

        await Promise.all([
            useCanvasStore.persist.rehydrate(),
            useCanvasHistoryStore.persist.rehydrate(),
            useAssetStore.persist.rehydrate(),
            useConfigStore.persist.rehydrate(),
            usePluginStore.persist.rehydrate(),
            useCreationPreferencesStore.persist.rehydrate(),
        ]);

        signal?.throwIfAborted();
        useConfigStore.getState().replaceConfig(localWorkspaceConfig(useConfigStore.getState().config));
        useCanvasStore.setState((state) => ({ projects: state.projects.map(normalizeLocalCanvasProject) }));
        useAssetStore.setState((state) => ({ assets: state.assets.map(normalizeLocalAsset) }));

        // Zustand keeps the previous in-memory value when a scoped snapshot is
        // absent, so every empty local namespace must be reset explicitly.
        if (!persistedCanvas) useCanvasStore.setState({ projects: [] });
        if (!persistedCanvasHistory) useCanvasHistoryStore.setState({ deletedProjects: [] });
        if (!persistedAssets) useAssetStore.setState({ assets: [] });
        if (usesWorkspaceAssetLibraryApi()) {
            try {
                await preserveLegacyCacheOnlyAssetDrafts(captureUserScope());
            } catch {
                // 查询失败不能当成「库是空的」。素材页会单独展示读取失败。
            }
        }
        if (!persistedPlugins) usePluginStore.setState({ installations: [], runtimeStatuses: {}, pluginStates: {} });
        if (!persistedCreationPreferences) useCreationPreferencesStore.setState({ preferences: {} });
        if (!persistedConfig) useConfigStore.getState().replaceConfig(localWorkspaceConfig(defaultConfig));
    } finally {
        if (!signal?.aborted) useUserStore.getState().setHydrated(true);
    }
}

/** Strip legacy hosted channel snapshots before local provider config is used. */
export function localWorkspaceConfig(config: AiConfig): AiConfig {
    return normalizeConfigSnapshot({
        config: {
            ...config,
            channels: config.channels.filter((channel) => channel.scope !== "system" && !isSystemProxyBaseUrl(channel.baseUrl)),
            runningHub: { ...defaultConfig.runningHub },
        },
    }).config;
}
