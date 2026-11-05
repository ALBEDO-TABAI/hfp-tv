import { getPlatformMe } from "@/services/api/platform";
import { getLocalModelConfig } from "@/services/api/workspace";
import { isPlatformManagedChannel, normalizeConfigSnapshot, useConfigStore, type ModelChannel } from "@/stores/use-config-store";
import { usePlatformSession } from "@/stores/use-platform-session";

/**
 * 公司中转站由团队服务器在服务端改写（绑定或清除密钥、管理员修改模型列表），浏览器只在打开页面时读一次模型配置。
 * 这里只把服务端的平台渠道换进当前配置，成员自己的渠道保持浏览器里的最新编辑，避免覆盖尚未保存完的改动。
 */
export async function refreshPlatformManagedChannels(): Promise<ModelChannel | undefined> {
    const server = await getLocalModelConfig();
    // 读不到完整配置时宁可不动，也不能把公司中转站当成“已被移除”。
    if (!server || typeof server !== "object" || !Array.isArray(server.config?.channels)) throw new Error("读取模型配置失败，请刷新页面");
    const managed = normalizeConfigSnapshot({ config: server.config }).config.channels.filter(isPlatformManagedChannel);
    const { config, replaceConfig } = useConfigStore.getState();
    const current = config.channels.filter(isPlatformManagedChannel);
    if (JSON.stringify(current) !== JSON.stringify(managed)) {
        const channels = [...managed, ...config.channels.filter((channel) => !isPlatformManagedChannel(channel))];
        replaceConfig(normalizeConfigSnapshot({ config: { ...config, channels } }).config);
    }
    return managed[0];
}

/** 管理员给自己绑定或清除中转站密钥后，账户状态和公司中转站渠道都要跟着更新。 */
export async function refreshOwnRelayBinding() {
    const [me] = await Promise.all([getPlatformMe(), refreshPlatformManagedChannels()]);
    usePlatformSession.getState().setMe(me);
}
