import { useState } from "react";
import { createRoot } from "react-dom/client";
import { App, ConfigProvider } from "antd";
import { getAntThemeConfig } from "../../src/lib/app-theme";
import { ModelServiceEditor } from "../../src/pages/settings/model-service-editor";
import { createModelChannel, defaultConfig, useConfigStore, type ModelChannel } from "../../src/stores/use-config-store";
import { ensureModelProfilesWithUiDefaults } from "../../src/lib/model-protocols";
import { ChannelSettingsPane } from "../../src/pages/settings/channel-settings-pane";
import { usePlatformSession } from "../../src/stores/use-platform-session";

const params = new URLSearchParams(location.search);
const companyRole = params.get("company");
if (companyRole) {
    usePlatformSession.setState({ mode: companyRole === "local" ? "local" : "hosted", me: {
        user: { id: "admin", username: "admin", displayName: "Admin", role: "admin" },
        workspace: { id: "workspace", status: "active" }, relay: { bound: false, hint: "" },
        capabilities: { hosted: true, externalAgents: false, storageSettings: false, eagle: false, depthCapture: false, pluginInstall: false, diagnostics: false, userAdmin: companyRole !== "member", skillReview: false, workshop: true, workshopReview: false },
    } });
    useConfigStore.setState({ config: { ...defaultConfig, channels: params.has("empty") ? [] : [createModelChannel({ id: "company-relay", pinned: true, name: "公司中转站", baseUrl: "http://10.10.10.45:3000/v1", models: ["seedance-2.5"], modelProfiles: [{ model: "seedance-2.5", capability: "video", protocol: "volcengine-ark-video" }] })] } });
}

function Harness() {
    const [saved, setSaved] = useState<ModelChannel>();
    const dark = new URLSearchParams(location.search).get("theme") === "dark";
    const inherited = new URLSearchParams(location.search).has("inherited");
    return <ConfigProvider theme={getAntThemeConfig(dark)}><App>
        {companyRole ? <ChannelSettingsPane /> : <ModelServiceEditor initial={createModelChannel({ name: "外接服务", baseUrl: "https://video.example.com/v1", apiKey: "fixture-key", models: inherited ? ["seedance-2.5"] : [], ...(inherited ? { interfaceType: "video-generations-json", modelProfiles: ensureModelProfilesWithUiDefaults(["seedance-2.5"], undefined) } : {}) })} onClose={() => {}} onSave={async (channel) => { setSaved(channel); }} />}
        <output data-testid="saved-profile">{saved?.modelProfiles?.[0]?.protocol || "unsaved"}</output>
    </App></ConfigProvider>;
}
createRoot(document.getElementById("root")!).render(<Harness />);
