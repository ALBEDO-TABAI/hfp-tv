import { createModelChannel, type ModelChannel } from "@/stores/use-config-store";
import type { RelayChannelProfile, RelayChannelTemplate } from "@/services/api/platform";

export type CompanyRelaySharedEndpoints = { arkBaseUrl: string; referenceAssetService: string };

/**
 * The editor draft for the company relay. `hasApiKey` is whether the admin's own
 * workspace holds a bound relay key: the server completes the admin's test
 * requests with it, so the draft carries it like a member's channel does.
 */
export function companyRelayChannelFromTemplate(template: RelayChannelTemplate | null, options: { hasApiKey?: boolean } = {}): ModelChannel {
    const channel = createModelChannel({ id: "company-relay", apiFormat: template?.apiFormat, pinned: true, hasApiKey: options.hasApiKey === true });
    delete channel.secretKey;
    delete channel.headers;
    return {
        ...channel,
        name: template?.name ?? "公司中转站",
        baseUrl: template?.baseUrl ?? "",
        apiKey: "",
        models: [...(template?.models ?? [])],
        modelProfiles: structuredClone(template?.modelProfiles ?? []),
    };
}

export function companyRelaySharedEndpoints(channel: ModelChannel): CompanyRelaySharedEndpoints {
    const profiles: RelayChannelProfile[] = channel.modelProfiles ?? [];
    return {
        arkBaseUrl: profiles.find(profile => profile.protocol?.startsWith("volcengine-ark-"))?.baseUrl ?? "",
        referenceAssetService: profiles.find(profile => profile.protocol === "volcengine-ark-video")?.referenceAssetService ?? "",
    };
}

/**
 * `previous` is what the shared fields showed when the editor opened. A shared
 * setting follows the field only on models that used that value or had none, so
 * a model the server file gave its own endpoint keeps it.
 */
export function companyRelayTemplateFromChannel(channel: ModelChannel, shared: CompanyRelaySharedEndpoints, previous: CompanyRelaySharedEndpoints = companyRelaySharedEndpoints(channel)): RelayChannelTemplate {
    if (channel.apiFormat !== "openai" && channel.apiFormat !== "gemini") throw new Error("公司中转站目录接口只支持 OpenAI 或 Gemini");
    const follows = (own: string | undefined, before: string) => !own?.trim() || own.trim() === before.trim();
    const profiles: RelayChannelProfile[] = channel.modelProfiles ?? [];
    const modelProfiles = profiles.filter(profile => channel.models.includes(profile.model)).map((profile): RelayChannelProfile => {
        const next: RelayChannelProfile = { ...profile };
        if (profile.protocol?.startsWith("volcengine-ark-") && follows(profile.baseUrl, previous.arkBaseUrl)) {
            if (shared.arkBaseUrl.trim()) next.baseUrl = shared.arkBaseUrl.trim();
            else delete next.baseUrl;
        }
        if (profile.protocol === "volcengine-ark-video" && follows(profile.referenceAssetService, previous.referenceAssetService)) {
            if (shared.referenceAssetService.trim()) next.referenceAssetService = shared.referenceAssetService.trim();
            else delete next.referenceAssetService;
        }
        return next;
    });
    return {
        name: channel.name.trim() || "公司中转站",
        baseUrl: channel.baseUrl.trim().replace(/\/+$/, ""),
        apiFormat: channel.apiFormat,
        models: [...channel.models],
        modelProfiles,
    };
}

/**
 * The address a model on the company relay is called on, mirroring the server:
 * a model's own endpoint is used only on the relay's origin, otherwise the relay
 * base URL. Tests run against the saved definition, so an editor test is only
 * meaningful while the draft resolves to the same address.
 */
export function companyRelayModelEndpoint(template: { baseUrl: string; modelProfiles?: RelayChannelProfile[] } | null, model: string) {
    if (!template) return "";
    const base = trimEndpoint(template.baseUrl);
    const own = trimEndpoint(template.modelProfiles?.find(profile => profile.model === model)?.baseUrl ?? "");
    return own && sameOrigin(own, base) ? own : base;
}

function trimEndpoint(value: string) {
    return value.trim().replace(/\/+$/, "");
}

function sameOrigin(left: string, right: string) {
    try {
        const a = new URL(left), b = new URL(right);
        return Boolean(a.host) && a.protocol === b.protocol && a.host.toLowerCase() === b.host.toLowerCase();
    } catch {
        return false;
    }
}
