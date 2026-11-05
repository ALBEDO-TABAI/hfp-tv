import { getPlatformMe } from "@/services/api/platform";
import { getLocalModelConfig } from "@/services/api/workspace";
import { isPlatformManagedChannel, normalizeConfigSnapshot, PLATFORM_RELAY_CHANNEL_ID, useConfigStore, type AiConfig, type ModelChannel } from "@/stores/use-config-store";
import { usePlatformSession } from "@/stores/use-platform-session";
import { getActiveUserScope } from "@/lib/user-scope";

// 服务端在公司中转站删掉某个模型时，会把指向它的默认选择改成仍可用的模型；这几项要跟着服务端走。
const SELECTION_FIELDS = ["model", "textModel", "assistantModel", "mediaReviewModel", "imageModel", "videoModel", "audioModel"] as const satisfies readonly (keyof AiConfig)[];

// 并发的刷新可能先发后到：同一账号下只采用不比已采用的更旧的配置版本。
let applied: { scope: string; revision: number } | null = null;

/**
 * 公司中转站由团队服务器在服务端改写（绑定或清除密钥、管理员修改模型列表），浏览器只在打开页面时读一次模型配置。
 * 这里只把服务端的平台渠道换进当前配置，成员自己的渠道保持浏览器里的最新编辑，避免覆盖尚未保存完的改动。
 */
export async function refreshPlatformManagedChannels(): Promise<ModelChannel | undefined> {
    const server = await getLocalModelConfig();
    // 读不到完整配置时宁可不动，也不能把公司中转站当成“已被移除”。
    if (!server || typeof server !== "object" || !Array.isArray(server.config?.channels)) throw new Error("读取模型配置失败，请刷新页面");
    const { config, replaceConfig } = useConfigStore.getState();
    const current = config.channels.filter(isPlatformManagedChannel);
    const scope = String(getActiveUserScope() ?? "");
    if (applied?.scope === scope && Number.isSafeInteger(server.revision) && server.revision < applied.revision) return current[0];
    if (Number.isSafeInteger(server.revision)) applied = { scope, revision: server.revision };
    const fresh = normalizeConfigSnapshot({ config: server.config }).config;
    const managed = fresh.channels.filter(isPlatformManagedChannel);
    const companyModels = new Set(managed.flatMap((channel) => channel.models));
    const selections: Partial<Pick<AiConfig, (typeof SELECTION_FIELDS)[number]>> = {};
    for (const field of SELECTION_FIELDS) {
        const value = config[field];
        const prefix = `${PLATFORM_RELAY_CHANNEL_ID}::`;
        if (typeof value === "string" && value.startsWith(prefix) && !companyModels.has(value.slice(prefix.length))) selections[field] = fresh[field] ?? "";
    }
    if (JSON.stringify(current) !== JSON.stringify(managed) || Object.keys(selections).length) {
        const channels = [...managed, ...config.channels.filter((channel) => !isPlatformManagedChannel(channel))];
        const next = normalizeConfigSnapshot({ config: { ...config, ...selections, channels } }).config;
        // Replacing triggers a save; skip it when nothing actually changes.
        if (JSON.stringify(next) !== JSON.stringify(config)) replaceConfig(next);
    }
    return managed[0];
}

/** 管理员给自己绑定或清除中转站密钥后，账户状态和公司中转站渠道都要跟着更新。 */
export async function refreshOwnRelayBinding() {
    const [me] = await Promise.all([getPlatformMe(), refreshPlatformManagedChannels()]);
    usePlatformSession.getState().setMe(me);
}

/** Tests only: forget the last applied configuration version. */
export function resetPlatformManagedChannelRefresh() {
    applied = null;
}
