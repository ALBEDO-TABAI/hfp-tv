import { createModelChannel, type ModelChannel } from "@/stores/use-config-store";
import type { RelayChannelProfile, RelayChannelTemplate } from "@/services/api/platform";

export type CompanyRelaySharedEndpoints = { arkBaseUrl: string; referenceAssetService: string };

export function companyRelayChannelFromTemplate(template: RelayChannelTemplate | null): ModelChannel {
    const channel = createModelChannel({ id: "company-relay", apiFormat: template?.apiFormat, pinned: true });
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

export function companyRelayTemplateFromChannel(channel: ModelChannel, shared: CompanyRelaySharedEndpoints): RelayChannelTemplate {
    if (channel.apiFormat !== "openai" && channel.apiFormat !== "gemini") throw new Error("公司中转站目录接口只支持 OpenAI 或 Gemini");
    const modelProfiles = (channel.modelProfiles ?? []).filter(profile => channel.models.includes(profile.model)).map((profile): RelayChannelProfile => {
        const next: RelayChannelProfile = { ...profile };
        if (profile.protocol?.startsWith("volcengine-ark-")) {
            if (shared.arkBaseUrl.trim()) next.baseUrl = shared.arkBaseUrl.trim();
            else delete next.baseUrl;
        }
        if (profile.protocol === "volcengine-ark-video") {
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
